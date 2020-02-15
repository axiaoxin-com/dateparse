// Package dateparse parses date-strings without knowing the format
// in advance, using a fast lex based approach to eliminate shotgun
// attempts.  It leans towards US style dates when there is a conflict.
package dateparse

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

//       _           _
//      | |         | |
//    __| |   __ _  | |_    ___   _ __     __ _   _ __   ___    ___
//   / _` |  / _` | | __|  / _ \ | '_ \   / _` | | '__| / __|  / _ \
//  | (_| | | (_| | | |_  |  __/ | |_) | | (_| | | |    \__ \ |  __/
//   \__,_|  \__,_|  \__|  \___| | .__/   \__,_| |_|    |___/  \___|
//                               | |
//                               |_|

type DateState int

const (
	StateStart DateState = iota
	StateDigit
	StateDigitDash
	StateDigitDashAlpha
	StateDigitDashWs
	StateDigitDashWsWs
	StateDigitDashWsWsAMPMMaybe
	StateDigitDashWsWsOffset
	StateDigitDashWsWsOffsetAlpha
	StateDigitDashWsWsOffsetColonAlpha
	StateDigitDashWsWsOffsetColon
	StateDigitDashWsOffset
	StateDigitDashWsWsAlpha
	StateDigitDashWsPeriod
	StateDigitDashWsPeriodAlpha
	StateDigitDashWsPeriodOffset
	StateDigitDashWsPeriodOffsetAlpha
	StateDigitDashT
	StateDigitDashTZ
	StateDigitDashTZDigit
	StateDigitDashTOffset
	StateDigitDashTOffsetColon
	StateDigitSlash
	StateDigitSlashWS
	StateDigitSlashWSColon
	StateDigitSlashWSColonAMPM
	StateDigitSlashWSColonColon
	StateDigitSlashWSColonColonAMPM
	StateDigitAlpha
	StateAlpha
	StateAlphaWS
	StateAlphaWSDigitComma
	StateAlphaWSAlpha
	StateAlphaWSAlphaColon
	StateAlphaWSAlphaColonOffset
	StateAlphaWSAlphaColonAlpha
	StateAlphaWSAlphaColonAlphaOffset
	StateAlphaWSAlphaColonAlphaOffsetAlpha
	StateWeekdayComma
	StateWeekdayCommaOffset
	StateWeekdayAbbrevComma
	StateWeekdayAbbrevCommaOffset
	StateWeekdayAbbrevCommaOffsetZone
	StateHowLongAgo
	StateTimestamp
	StateNow
)

const (
	Day = time.Hour * 24
)

var (
	shortDates = []string{"01/02/2006", "1/2/2006", "06/01/02", "01/02/06", "1/2/06"}
)

// ParseAny parse an unknown date format, detect the layout, parse.
// Normal parse.  Equivalent Timezone rules as time.Parse()
func ParseAny(datestr string) (time.Time, DateState, error) {
	return parseTime(datestr, nil)
}

// ParseIn with Location, equivalent to time.ParseInLocation() timezone/offset
// rules.  Using location arg, if timezone/offset info exists in the
// datestring, it uses the given location rules for any zone interpretation.
// That is, MST means one thing when using America/Denver and something else
// in other locations.
func ParseIn(datestr string, loc *time.Location) (time.Time, DateState, error) {
	return parseTime(datestr, loc)
}

// ParseLocal Given an unknown date format, detect the layout,
// using time.Local, parse.
//
// Set Location to time.Local.  Same as ParseIn Location but lazily uses
// the global time.Local variable for Location argument.
//
//     denverLoc, _ := time.LoadLocation("America/Denver")
//     time.Local = denverLoc
//
//     t, err := dateparse.ParseLocal("3/1/2014")
//
// Equivalent to:
//
//     t, err := dateparse.ParseIn("3/1/2014", denverLoc)
//
func ParseLocal(datestr string) (time.Time, DateState, error) {
	return parseTime(datestr, time.Local)
}

// MustParse  parse a date, and panic if it can't be parsed.  Used for testing.
// Not recommended for most use-cases.
func MustParse(datestr string) time.Time {
	t, _, err := parseTime(datestr, nil)
	if err != nil {
		panic(err.Error())
	}
	return t
}

func parse(layout, datestr string, loc *time.Location) (time.Time, error) {
	if loc == nil {
		return time.Parse(layout, datestr)
	}
	return time.ParseInLocation(layout, datestr, loc)
}

func parseTime(datestr string, loc *time.Location) (time.Time, DateState, error) {
	if strings.ToLower(datestr) == "now" {
		return time.Now(), StateNow, nil
	}

	state := StateStart

	firstSlash := 0

	// General strategy is to read rune by rune through the date looking for
	// certain hints of what type of date we are dealing with.
	// Hopefully we only need to read about 5 or 6 bytes before
	// we figure it out and then attempt a parse
iterRunes:
	for i := 0; i < len(datestr); i++ {
		r := rune(datestr[i])
		// r, bytesConsumed := utf8.DecodeRuneInString(datestr[ri:])
		// if bytesConsumed > 1 {
		// 	ri += (bytesConsumed - 1)
		// }

		switch state {
		case StateStart:
			if unicode.IsDigit(r) {
				state = StateDigit
			} else if unicode.IsLetter(r) {
				state = StateAlpha
			}
		case StateDigit: // starts digits
			if unicode.IsDigit(r) {
				continue
			} else if unicode.IsLetter(r) {
				state = StateDigitAlpha
				continue
			}
			switch r {
			case '-', '\u2212':
				state = StateDigitDash
			case '/':
				state = StateDigitSlash
				firstSlash = i
			}
		case StateDigitDash: // starts digit then dash 02-
			// 2006-01-02T15:04:05Z07:00
			// 2017-06-25T17:46:57.45706582-07:00
			// 2006-01-02T15:04:05.999999999Z07:00
			// 2006-01-02T15:04:05+0000
			// 2012-08-03 18:31:59.257000000
			// 2014-04-26 17:24:37.3186369
			// 2017-01-27 00:07:31.945167
			// 2016-03-14 00:00:00.000
			// 2014-05-11 08:20:13,787
			// 2017-07-19 03:21:51+00:00
			// 2006-01-02
			// 2013-04-01 22:43:22
			// 2014-04-26 05:24:37 PM
			// 2013-Feb-03
			switch {
			case r == ' ':
				state = StateDigitDashWs
			case r == 'T':
				state = StateDigitDashT
			default:
				if unicode.IsLetter(r) {
					state = StateDigitDashAlpha
					break iterRunes
				}
			}
		case StateDigitDashWs:
			// 2013-04-01 22:43:22
			// 2014-05-11 08:20:13,787
			// stateDigitDashWsWs
			//   2014-04-26 05:24:37 PM
			//   2014-12-16 06:20:00 UTC
			//   2015-02-18 00:12:00 +0000 UTC
			//   2006-01-02 15:04:05 -0700
			//   2006-01-02 15:04:05 -07:00
			// stateDigitDashWsOffset
			//   2017-07-19 03:21:51+00:00
			// stateDigitDashWsPeriod
			//   2014-04-26 17:24:37.3186369
			//   2017-01-27 00:07:31.945167
			//   2012-08-03 18:31:59.257000000
			//   2016-03-14 00:00:00.000
			//   stateDigitDashWsPeriodOffset
			//     2017-01-27 00:07:31.945167 +0000
			//     2016-03-14 00:00:00.000 +0000
			//     stateDigitDashWsPeriodOffsetAlpha
			//       2017-01-27 00:07:31.945167 +0000 UTC
			//       2016-03-14 00:00:00.000 +0000 UTC
			//   stateDigitDashWsPeriodAlpha
			//     2014-12-16 06:20:00.000 UTC
			switch r {
			case ',':
				if len(datestr) == len("2014-05-11 08:20:13,787") {
					// go doesn't seem to parse this one natively?   or did i miss it?
					t, err := parse("2006-01-02 03:04:05", datestr[:i], loc)
					if err == nil {
						ms, err := strconv.Atoi(datestr[i+1:])
						if err == nil {
							return time.Unix(0, t.UnixNano()+int64(ms)*1e6), StateDigitDashWs, nil
						}
					}
					return t, StateDigitDashWs, err
				}
			case '-', '+':
				state = StateDigitDashWsOffset
			case '.':
				state = StateDigitDashWsPeriod
			case ' ':
				state = StateDigitDashWsWs
			}

		case StateDigitDashWsWs:
			// stateDigitDashWsWsAlpha
			//   2014-12-16 06:20:00 UTC
			//   stateDigitDashWsWsAMPMMaybe
			//     2014-04-26 05:24:37 PM
			// stateDigitDashWsWsOffset
			//   2006-01-02 15:04:05 -0700
			//   stateDigitDashWsWsOffsetColon
			//     2006-01-02 15:04:05 -07:00
			//     stateDigitDashWsWsOffsetColonAlpha
			//       2015-02-18 00:12:00 +00:00 UTC
			//   stateDigitDashWsWsOffsetAlpha
			//     2015-02-18 00:12:00 +0000 UTC
			switch r {
			case 'A', 'P':
				state = StateDigitDashWsWsAMPMMaybe
			case '+', '-':
				state = StateDigitDashWsWsOffset
			default:
				if unicode.IsLetter(r) {
					// 2014-12-16 06:20:00 UTC
					state = StateDigitDashWsWsAlpha
					break iterRunes
				}
			}

		case StateDigitDashWsWsAMPMMaybe:
			if r == 'M' {
				t, err := parse("2006-01-02 03:04:05 PM", datestr, loc)
				return t, StateDigitDashWsWsAMPMMaybe, err
			}
			state = StateDigitDashWsWsAlpha

		case StateDigitDashWsWsOffset:
			// stateDigitDashWsWsOffset
			//   2006-01-02 15:04:05 -0700
			//   stateDigitDashWsWsOffsetColon
			//     2006-01-02 15:04:05 -07:00
			//     stateDigitDashWsWsOffsetColonAlpha
			//       2015-02-18 00:12:00 +00:00 UTC
			//   stateDigitDashWsWsOffsetAlpha
			//     2015-02-18 00:12:00 +0000 UTC
			if r == ':' {
				state = StateDigitDashWsWsOffsetColon
			} else if unicode.IsLetter(r) {
				// 2015-02-18 00:12:00 +0000 UTC
				state = StateDigitDashWsWsOffsetAlpha
				break iterRunes
			}

		case StateDigitDashWsWsOffsetColon:
			// stateDigitDashWsWsOffsetColon
			//   2006-01-02 15:04:05 -07:00
			//   stateDigitDashWsWsOffsetColonAlpha
			//     2015-02-18 00:12:00 +00:00 UTC
			if unicode.IsLetter(r) {
				// 2015-02-18 00:12:00 +00:00 UTC
				state = StateDigitDashWsWsOffsetColonAlpha
				break iterRunes
			}

		case StateDigitDashWsPeriod:
			// 2014-04-26 17:24:37.3186369
			// 2017-01-27 00:07:31.945167
			// 2012-08-03 18:31:59.257000000
			// 2016-03-14 00:00:00.000
			// stateDigitDashWsPeriodOffset
			//   2017-01-27 00:07:31.945167 +0000
			//   2016-03-14 00:00:00.000 +0000
			//   stateDigitDashWsPeriodOffsetAlpha
			//     2017-01-27 00:07:31.945167 +0000 UTC
			//     2016-03-14 00:00:00.000 +0000 UTC
			// stateDigitDashWsPeriodAlpha
			//   2014-12-16 06:20:00.000 UTC
			if unicode.IsLetter(r) {
				// 2014-12-16 06:20:00.000 UTC
				state = StateDigitDashWsPeriodAlpha
				break iterRunes
			} else if r == '+' || r == '-' {
				state = StateDigitDashWsPeriodOffset
			}
		case StateDigitDashWsPeriodOffset:
			// 2017-01-27 00:07:31.945167 +0000
			// 2016-03-14 00:00:00.000 +0000
			// stateDigitDashWsPeriodOffsetAlpha
			//   2017-01-27 00:07:31.945167 +0000 UTC
			//   2016-03-14 00:00:00.000 +0000 UTC
			if unicode.IsLetter(r) {
				// 2014-12-16 06:20:00.000 UTC
				// 2017-01-27 00:07:31.945167 +0000 UTC
				// 2016-03-14 00:00:00.000 +0000 UTC
				state = StateDigitDashWsPeriodOffsetAlpha
				break iterRunes
			}
		case StateDigitDashT: // starts digit then dash 02-  then T
			// stateDigitDashT
			// 2006-01-02T15:04:05
			// stateDigitDashTZ
			// 2006-01-02T15:04:05.999999999Z
			// 2006-01-02T15:04:05.99999999Z
			// 2006-01-02T15:04:05.9999999Z
			// 2006-01-02T15:04:05.999999Z
			// 2006-01-02T15:04:05.99999Z
			// 2006-01-02T15:04:05.9999Z
			// 2006-01-02T15:04:05.999Z
			// 2006-01-02T15:04:05.99Z
			// 2009-08-12T22:15Z
			// stateDigitDashTZDigit
			// 2006-01-02T15:04:05.999999999Z07:00
			// 2006-01-02T15:04:05Z07:00
			// With another dash aka time-zone at end
			// stateDigitDashTOffset
			//   stateDigitDashTOffsetColon
			//     2017-06-25T17:46:57.45706582-07:00
			//     2017-06-25T17:46:57+04:00
			// 2006-01-02T15:04:05+0000
			switch r {
			case '-', '+':
				state = StateDigitDashTOffset
			case 'Z':
				state = StateDigitDashTZ
			}
		case StateDigitDashTZ:
			if unicode.IsDigit(r) {
				state = StateDigitDashTZDigit
			}
		case StateDigitDashTOffset:
			if r == ':' {
				state = StateDigitDashTOffsetColon
			}
		case StateDigitSlash: // starts digit then slash 02/
			// 2014/07/10 06:55:38.156283
			// 03/19/2012 10:11:59
			// 04/2/2014 03:00:37
			// 3/1/2012 10:11:59
			// 4/8/2014 22:05
			// 3/1/2014
			// 10/13/2014
			// 01/02/2006
			// 1/2/06
			if unicode.IsDigit(r) || r == '/' {
				continue
			}
			switch r {
			case ' ':
				state = StateDigitSlashWS
			}
		case StateDigitSlashWS: // starts digit then slash 02/ more digits/slashes then whitespace
			// 2014/07/10 06:55:38.156283
			// 03/19/2012 10:11:59
			// 04/2/2014 03:00:37
			// 3/1/2012 10:11:59
			// 4/8/2014 22:05
			switch r {
			case ':':
				state = StateDigitSlashWSColon
			}
		case StateDigitSlashWSColon: // starts digit then slash 02/ more digits/slashes then whitespace
			// 2014/07/10 06:55:38.156283
			// 03/19/2012 10:11:59
			// 04/2/2014 03:00:37
			// 3/1/2012 10:11:59
			// 4/8/2014 22:05
			// 3/1/2012 10:11:59 AM
			switch r {
			case ':':
				state = StateDigitSlashWSColonColon
			case 'A', 'P':
				state = StateDigitSlashWSColonAMPM
			}
		case StateDigitSlashWSColonColon: // starts digit then slash 02/ more digits/slashes then whitespace
			// 2014/07/10 06:55:38.156283
			// 03/19/2012 10:11:59
			// 04/2/2014 03:00:37
			// 3/1/2012 10:11:59
			// 4/8/2014 22:05
			// 3/1/2012 10:11:59 AM
			switch r {
			case 'A', 'P':
				state = StateDigitSlashWSColonColonAMPM
			}
		case StateDigitAlpha:
			// 12 Feb 2006, 19:17
			// 12 Feb 2006, 19:17:22
			switch {
			case len(datestr) == len("02 Jan 2006, 15:04"):
				t, err := parse("02 Jan 2006, 15:04", datestr, loc)
				return t, StateDigitAlpha, err
			case len(datestr) == len("02 Jan 2006, 15:04:05"):
				t, err := parse("02 Jan 2006, 15:04:05", datestr, loc)
				return t, StateDigitAlpha, err
			case len(datestr) == len("2006年01月02日"):
				t, err := parse("2006年01月02日", datestr, loc)
				return t, StateDigitAlpha, err
			case len(datestr) == len("2006年01月02日 15:04"):
				t, err := parse("2006年01月02日 15:04", datestr, loc)
				return t, StateDigitAlpha, err
			case strings.Contains(datestr, "ago"):
				state = StateHowLongAgo
			}
		case StateAlpha: // starts alpha
			// stateAlphaWS
			//  Mon Jan _2 15:04:05 2006
			//  Mon Jan _2 15:04:05 MST 2006
			//  Mon Jan 02 15:04:05 -0700 2006
			//  Mon Aug 10 15:44:11 UTC+0100 2015
			//  Fri Jul 03 2015 18:04:07 GMT+0100 (GMT Daylight Time)
			//  stateAlphaWSDigitComma
			//    May 8, 2009 5:57:51 PM
			//
			// stateWeekdayComma
			//   Monday, 02-Jan-06 15:04:05 MST
			//   stateWeekdayCommaOffset
			//     Monday, 02 Jan 2006 15:04:05 -0700
			//     Monday, 02 Jan 2006 15:04:05 +0100
			// stateWeekdayAbbrevComma
			//   Mon, 02-Jan-06 15:04:05 MST
			//   Mon, 02 Jan 2006 15:04:05 MST
			//   stateWeekdayAbbrevCommaOffset
			//     Mon, 02 Jan 2006 15:04:05 -0700
			//     Thu, 13 Jul 2017 08:58:40 +0100
			//     stateWeekdayAbbrevCommaOffsetZone
			//       Tue, 11 Jul 2017 16:28:13 +0200 (CEST)
			switch {
			case unicode.IsLetter(r):
				continue
			case r == ' ':
				state = StateAlphaWS
			case r == ',':
				if i == 3 {
					state = StateWeekdayAbbrevComma
				} else {
					state = StateWeekdayComma
				}
			}
		case StateWeekdayComma: // Starts alpha then comma
			// Mon, 02-Jan-06 15:04:05 MST
			// Mon, 02 Jan 2006 15:04:05 MST
			// stateWeekdayCommaOffset
			//   Monday, 02 Jan 2006 15:04:05 -0700
			//   Monday, 02 Jan 2006 15:04:05 +0100
			switch {
			case r == '-':
				if i < 15 {
					t, err := parse("Monday, 02-Jan-06 15:04:05 MST", datestr, loc)
					return t, StateWeekdayComma, err
				}
				state = StateWeekdayCommaOffset
			case r == '+':
				state = StateWeekdayCommaOffset
			}
		case StateWeekdayAbbrevComma: // Starts alpha then comma
			// Mon, 02-Jan-06 15:04:05 MST
			// Mon, 02 Jan 2006 15:04:05 MST
			// stateWeekdayAbbrevCommaOffset
			//   Mon, 02 Jan 2006 15:04:05 -0700
			//   Thu, 13 Jul 2017 08:58:40 +0100
			//   stateWeekdayAbbrevCommaOffsetZone
			//     Tue, 11 Jul 2017 16:28:13 +0200 (CEST)
			switch {
			case r == '-':
				if i < 15 {
					t, err := parse("Mon, 02-Jan-06 15:04:05 MST", datestr, loc)
					return t, StateWeekdayAbbrevComma, err
				}
				state = StateWeekdayAbbrevCommaOffset
			case r == '+':
				state = StateWeekdayAbbrevCommaOffset
			}

		case StateWeekdayAbbrevCommaOffset:
			// stateWeekdayAbbrevCommaOffset
			//   Mon, 02 Jan 2006 15:04:05 -0700
			//   Thu, 13 Jul 2017 08:58:40 +0100
			//   stateWeekdayAbbrevCommaOffsetZone
			//     Tue, 11 Jul 2017 16:28:13 +0200 (CEST)
			if r == '(' {
				state = StateWeekdayAbbrevCommaOffsetZone
			}

		case StateAlphaWS: // Starts alpha then whitespace
			// Mon Jan _2 15:04:05 2006
			// Mon Jan _2 15:04:05 MST 2006
			// Mon Jan 02 15:04:05 -0700 2006
			// Fri Jul 03 2015 18:04:07 GMT+0100 (GMT Daylight Time)
			// Mon Aug 10 15:44:11 UTC+0100 2015
			switch {
			case unicode.IsLetter(r):
				state = StateAlphaWSAlpha
			case unicode.IsDigit(r):
				state = StateAlphaWSDigitComma
			}

		case StateAlphaWSDigitComma: // Starts Alpha, whitespace, digit, comma
			// May 8, 2009 5:57:51 PM
			// May 8, 2009
			if len(datestr) == len("May 8, 2009") {
				t, err := parse("Jan 2, 2006", datestr, loc)
				return t, StateAlphaWSDigitComma, err
			}
			t, err := parse("Jan 2, 2006 3:04:05 PM", datestr, loc)
			return t, StateAlphaWSDigitComma, err

		case StateAlphaWSAlpha: // Alpha, whitespace, alpha
			// Mon Jan _2 15:04:05 2006
			// Mon Jan 02 15:04:05 -0700 2006
			// Mon Jan _2 15:04:05 MST 2006
			// Mon Aug 10 15:44:11 UTC+0100 2015
			// Fri Jul 03 2015 18:04:07 GMT+0100 (GMT Daylight Time)
			if r == ':' {
				state = StateAlphaWSAlphaColon
			}
		case StateAlphaWSAlphaColon: // Alpha, whitespace, alpha, :
			// Mon Jan _2 15:04:05 2006
			// Mon Jan 02 15:04:05 -0700 2006
			// Mon Jan _2 15:04:05 MST 2006
			// Mon Aug 10 15:44:11 UTC+0100 2015
			// Fri Jul 03 2015 18:04:07 GMT+0100 (GMT Daylight Time)
			if unicode.IsLetter(r) {
				state = StateAlphaWSAlphaColonAlpha
			} else if r == '-' || r == '+' {
				state = StateAlphaWSAlphaColonOffset
			}
		case StateAlphaWSAlphaColonAlpha: // Alpha, whitespace, alpha, :, alpha
			// Mon Jan _2 15:04:05 MST 2006
			// Mon Aug 10 15:44:11 UTC+0100 2015
			// Fri Jul 03 2015 18:04:07 GMT+0100 (GMT Daylight Time)
			if r == '+' {
				state = StateAlphaWSAlphaColonAlphaOffset
			}
		case StateAlphaWSAlphaColonAlphaOffset: // Alpha, whitespace, alpha, : , alpha, offset, ?
			// Mon Aug 10 15:44:11 UTC+0100 2015
			// Fri Jul 03 2015 18:04:07 GMT+0100 (GMT Daylight Time)
			if unicode.IsLetter(r) {
				state = StateAlphaWSAlphaColonAlphaOffsetAlpha
			}
		default:
			break iterRunes
		}
	}

	switch state {
	case StateDigit:
		// unixy timestamps ish
		//  1499979655583057426  nanoseconds
		//  1499979795437000     micro-seconds
		//  1499979795437        milliseconds
		//  1384216367189
		//  1332151919           seconds
		//  20140601             yyyymmdd
		//  2014                 yyyy
		t := time.Time{}
		if len(datestr) > len("1499979795437000") {
			if nanoSecs, err := strconv.ParseInt(datestr, 10, 64); err == nil {
				t = time.Unix(0, nanoSecs)
			}
		} else if len(datestr) > len("1499979795437") {
			if microSecs, err := strconv.ParseInt(datestr, 10, 64); err == nil {
				t = time.Unix(0, microSecs*1000)
			}
		} else if len(datestr) > len("1332151919") {
			if miliSecs, err := strconv.ParseInt(datestr, 10, 64); err == nil {
				t = time.Unix(0, miliSecs*1000*1000)
			}
		} else if len(datestr) == len("20140601") {
			t, err := parse("20060102", datestr, loc)
			return t, StateDigit, err
		} else if len(datestr) == len("2014") {
			t, err := parse("2006", datestr, loc)
			return t, StateDigit, err
		}
		if t.IsZero() {
			if secs, err := strconv.ParseInt(datestr, 10, 64); err == nil {
				if secs < 0 {
					// Now, for unix-seconds we aren't going to guess a lot
					// nothing before unix-epoch
				} else {
					t = time.Unix(secs, 0)
				}
			}
		}
		if !t.IsZero() {
			if loc == nil {
				return t, StateTimestamp, nil
			}
			return t.In(loc), StateTimestamp, nil
		}

	case StateDigitDash: // starts digit then dash 02-
		// 2006-01-02
		// 2006-01
		if len(datestr) == len("2014-04-26") {
			t, err := parse("2006-01-02", datestr, loc)
			return t, StateDigitDash, err
		} else if len(datestr) == len("2014-04") {
			t, err := parse("2006-01", datestr, loc)
			return t, StateDigitDash, err
		}
	case StateDigitDashAlpha:
		// 2013-Feb-03
		t, err := parse("2006-Jan-02", datestr, loc)
		return t, StateDigitDashAlpha, err

	case StateDigitDashTOffset:
		// 2006-01-02T15:04:05+0000
		t, err := parse("2006-01-02T15:04:05-0700", datestr, loc)
		return t, StateDigitDashTOffset, err

	case StateDigitDashTOffsetColon:
		// With another +/- time-zone at end
		// 2006-01-02T15:04:05.999999999+07:00
		// 2006-01-02T15:04:05.999999999-07:00
		// 2006-01-02T15:04:05.999999+07:00
		// 2006-01-02T15:04:05.999999-07:00
		// 2006-01-02T15:04:05.999+07:00
		// 2006-01-02T15:04:05.999-07:00
		// 2006-01-02T15:04:05+07:00
		// 2006-01-02T15:04:05-07:00
		t, err := parse("2006-01-02T15:04:05-07:00", datestr, loc)
		return t, StateDigitDashTOffsetColon, err

	case StateDigitDashT: // starts digit then dash 02-  then T
		// 2006-01-02T15:04:05.999999
		// 2006-01-02T15:04:05.999999
		t, err := parse("2006-01-02T15:04:05", datestr, loc)
		return t, StateDigitDashT, err

	case StateDigitDashTZDigit:
		// With a time-zone at end after Z
		// 2006-01-02T15:04:05.999999999Z07:00
		// 2006-01-02T15:04:05Z07:00
		// RFC3339     = "2006-01-02T15:04:05Z07:00"
		// RFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"
		return time.Time{}, StateDigitDashTZDigit, fmt.Errorf("RFC339 Dates may not contain both Z & Offset for %q see https://github.com/golang/go/issues/5294", datestr)

	case StateDigitDashTZ: // starts digit then dash 02-  then T Then Z
		// 2006-01-02T15:04:05.999999999Z
		// 2006-01-02T15:04:05.99999999Z
		// 2006-01-02T15:04:05.9999999Z
		// 2006-01-02T15:04:05.999999Z
		// 2006-01-02T15:04:05.99999Z
		// 2006-01-02T15:04:05.9999Z
		// 2006-01-02T15:04:05.999Z
		// 2006-01-02T15:04:05.99Z
		// 2009-08-12T22:15Z  -- No seconds/milliseconds
		switch len(datestr) {
		case len("2009-08-12T22:15Z"):
			t, err := parse("2006-01-02T15:04Z", datestr, loc)
			return t, StateDigitDashTZ, err
		default:
			t, err := parse("2006-01-02T15:04:05Z", datestr, loc)
			return t, StateDigitDashTZ, err
		}
	case StateDigitDashWs: // starts digit then dash 02-  then whitespace   1 << 2  << 5 + 3
		// 2013-04-01 22:43:22
		t, err := parse("2006-01-02 15:04:05", datestr, loc)
		return t, StateDigitDashWs, err

	case StateDigitDashWsWsOffset:
		// 2006-01-02 15:04:05 -0700
		t, err := parse("2006-01-02 15:04:05 -0700", datestr, loc)
		return t, StateDigitDashWsWsOffset, err

	case StateDigitDashWsWsOffsetColon:
		// 2006-01-02 15:04:05 -07:00
		t, err := parse("2006-01-02 15:04:05 -07:00", datestr, loc)
		return t, StateDigitDashWsWsOffsetColon, err

	case StateDigitDashWsWsOffsetAlpha:
		// 2015-02-18 00:12:00 +0000 UTC
		t, err := parse("2006-01-02 15:04:05 -0700 UTC", datestr, loc)
		if err == nil {
			return t, StateDigitDashWsWsOffsetAlpha, nil
		}
		t, err = parse("2006-01-02 15:04:05 +0000 GMT", datestr, loc)
		return t, StateDigitDashWsWsOffsetAlpha, nil

	case StateDigitDashWsWsOffsetColonAlpha:
		// 2015-02-18 00:12:00 +00:00 UTC
		t, err := parse("2006-01-02 15:04:05 -07:00 UTC", datestr, loc)
		return t, StateDigitDashWsWsOffsetColonAlpha, err

	case StateDigitDashWsOffset:
		// 2017-07-19 03:21:51+00:00
		t, err := parse("2006-01-02 15:04:05-07:00", datestr, loc)
		return t, StateDigitDashWsOffset, err

	case StateDigitDashWsWsAlpha:
		// 2014-12-16 06:20:00 UTC
		t, err := parse("2006-01-02 15:04:05 UTC", datestr, loc)
		if err == nil {
			return t, StateDigitDashWsWsAlpha, nil
		}
		t, err = parse("2006-01-02 15:04:05 GMT", datestr, loc)
		if err == nil {
			return t, StateDigitDashWsWsAlpha, nil
		}
		if len(datestr) > len("2006-01-02 03:04:05") {
			t, err = parse("2006-01-02 03:04:05", datestr[:len("2006-01-02 03:04:05")], loc)
			if err == nil {
				return t, StateDigitDashWsWsAlpha, nil
			}
		}

	case StateDigitDashWsPeriod:
		// 2012-08-03 18:31:59.257000000
		// 2014-04-26 17:24:37.3186369
		// 2017-01-27 00:07:31.945167
		// 2016-03-14 00:00:00.000
		t, err := parse("2006-01-02 15:04:05", datestr, loc)
		return t, StateDigitDashWsPeriod, err

	case StateDigitDashWsPeriodAlpha:
		// 2012-08-03 18:31:59.257000000 UTC
		// 2014-04-26 17:24:37.3186369 UTC
		// 2017-01-27 00:07:31.945167 UTC
		// 2016-03-14 00:00:00.000 UTC
		t, err := parse("2006-01-02 15:04:05 UTC", datestr, loc)
		return t, StateDigitDashWsPeriodAlpha, err

	case StateDigitDashWsPeriodOffset:
		// 2012-08-03 18:31:59.257000000 +0000
		// 2014-04-26 17:24:37.3186369 +0000
		// 2017-01-27 00:07:31.945167 +0000
		// 2016-03-14 00:00:00.000 +0000
		t, err := parse("2006-01-02 15:04:05 -0700", datestr, loc)
		return t, StateDigitDashWsPeriodOffset, err

	case StateDigitDashWsPeriodOffsetAlpha:
		// 2012-08-03 18:31:59.257000000 +0000 UTC
		// 2014-04-26 17:24:37.3186369 +0000 UTC
		// 2017-01-27 00:07:31.945167 +0000 UTC
		// 2016-03-14 00:00:00.000 +0000 UTC
		t, err := parse("2006-01-02 15:04:05 -0700 UTC", datestr, loc)
		return t, StateDigitDashWsPeriodOffsetAlpha, err

	case StateAlphaWSAlphaColon:
		// Mon Jan _2 15:04:05 2006
		t, err := parse(time.ANSIC, datestr, loc)
		return t, StateAlphaWSAlphaColon, err

	case StateAlphaWSAlphaColonOffset:
		// Mon Jan 02 15:04:05 -0700 2006
		t, err := parse(time.RubyDate, datestr, loc)
		return t, StateAlphaWSAlphaColonOffset, err

	case StateAlphaWSAlphaColonAlpha:
		// Mon Jan _2 15:04:05 MST 2006
		t, err := parse(time.UnixDate, datestr, loc)
		return t, StateAlphaWSAlphaColonAlpha, err

	case StateAlphaWSAlphaColonAlphaOffset:
		// Mon Aug 10 15:44:11 UTC+0100 2015
		t, err := parse("Mon Jan 02 15:04:05 MST-0700 2006", datestr, loc)
		return t, StateAlphaWSAlphaColonAlphaOffset, err

	case StateAlphaWSAlphaColonAlphaOffsetAlpha:
		// Fri Jul 03 2015 18:04:07 GMT+0100 (GMT Daylight Time)
		if len(datestr) > len("Mon Jan 02 2006 15:04:05 MST-0700") {
			// What effing time stamp is this?
			// Fri Jul 03 2015 18:04:07 GMT+0100 (GMT Daylight Time)
			dateTmp := datestr[:33]
			t, err := parse("Mon Jan 02 2006 15:04:05 MST-0700", dateTmp, loc)
			return t, StateAlphaWSAlphaColonAlphaOffsetAlpha, err
		}
	case StateDigitSlash: // starts digit then slash 02/ (but nothing else)
		// 3/1/2014
		// 10/13/2014
		// 01/02/2006
		// 2014/10/13
		if firstSlash == 4 {
			if len(datestr) == len("2006/01/02") {
				t, err := parse("2006/01/02", datestr, loc)
				return t, StateDigitSlash, err
			}
			t, err := parse("2006/1/2", datestr, loc)
			return t, StateDigitSlash, err
		}
		for _, parseFormat := range shortDates {
			if t, err := parse(parseFormat, datestr, loc); err == nil {
				return t, StateDigitSlash, nil
			}
		}

	case StateDigitSlashWSColon: // starts digit then slash 02/ more digits/slashes then whitespace
		// 4/8/2014 22:05
		// 04/08/2014 22:05
		// 2014/4/8 22:05
		// 2014/04/08 22:05

		if firstSlash == 4 {
			for _, layout := range []string{"2006/01/02 15:04", "2006/1/2 15:04", "2006/01/2 15:04", "2006/1/02 15:04"} {
				if t, err := parse(layout, datestr, loc); err == nil {
					return t, StateDigitSlashWSColon, nil
				}
			}
		} else {
			for _, layout := range []string{"01/02/2006 15:04", "01/2/2006 15:04", "1/02/2006 15:04", "1/2/2006 15:04"} {
				if t, err := parse(layout, datestr, loc); err == nil {
					return t, StateDigitSlashWSColon, nil
				}
			}
		}

	case StateDigitSlashWSColonAMPM: // starts digit then slash 02/ more digits/slashes then whitespace
		// 4/8/2014 22:05 PM
		// 04/08/2014 22:05 PM
		// 04/08/2014 1:05 PM
		// 2014/4/8 22:05 PM
		// 2014/04/08 22:05 PM

		if firstSlash == 4 {
			for _, layout := range []string{"2006/01/02 03:04 PM", "2006/01/2 03:04 PM", "2006/1/02 03:04 PM", "2006/1/2 03:04 PM",
				"2006/01/02 3:04 PM", "2006/01/2 3:04 PM", "2006/1/02 3:04 PM", "2006/1/2 3:04 PM"} {
				if t, err := parse(layout, datestr, loc); err == nil {
					return t, StateDigitSlashWSColonAMPM, nil
				}
			}
		} else {
			for _, layout := range []string{"01/02/2006 03:04 PM", "01/2/2006 03:04 PM", "1/02/2006 03:04 PM", "1/2/2006 03:04 PM",
				"01/02/2006 3:04 PM", "01/2/2006 3:04 PM", "1/02/2006 3:04 PM", "1/2/2006 3:04 PM"} {
				if t, err := parse(layout, datestr, loc); err == nil {
					return t, StateDigitSlashWSColonAMPM, nil
				}

			}
		}

	case StateDigitSlashWSColonColon: // starts digit then slash 02/ more digits/slashes then whitespace double colons
		// 2014/07/10 06:55:38.156283
		// 03/19/2012 10:11:59
		// 3/1/2012 10:11:59
		// 03/1/2012 10:11:59
		// 3/01/2012 10:11:59
		if firstSlash == 4 {
			for _, layout := range []string{"2006/01/02 15:04:05", "2006/1/02 15:04:05", "2006/01/2 15:04:05", "2006/1/2 15:04:05"} {
				if t, err := parse(layout, datestr, loc); err == nil {
					return t, StateDigitSlashWSColonColon, nil
				}
			}
		} else {
			for _, layout := range []string{"01/02/2006 15:04:05", "1/02/2006 15:04:05", "01/2/2006 15:04:05", "1/2/2006 15:04:05"} {
				if t, err := parse(layout, datestr, loc); err == nil {
					return t, StateDigitSlashWSColonColon, nil
				}
			}
		}

	case StateDigitSlashWSColonColonAMPM: // starts digit then slash 02/ more digits/slashes then whitespace double colons
		// 2014/07/10 06:55:38.156283 PM
		// 03/19/2012 10:11:59 PM
		// 3/1/2012 10:11:59 PM
		// 03/1/2012 10:11:59 PM
		// 3/01/2012 10:11:59 PM

		if firstSlash == 4 {
			for _, layout := range []string{"2006/01/02 03:04:05 PM", "2006/1/02 03:04:05 PM", "2006/01/2 03:04:05 PM", "2006/1/2 03:04:05 PM",
				"2006/01/02 3:04:05 PM", "2006/1/02 3:04:05 PM", "2006/01/2 3:04:05 PM", "2006/1/2 3:04:05 PM"} {
				if t, err := parse(layout, datestr, loc); err == nil {
					return t, StateDigitSlashWSColonColonAMPM, nil
				}
			}
		} else {
			for _, layout := range []string{"01/02/2006 03:04:05 PM", "1/02/2006 03:04:05 PM", "01/2/2006 03:04:05 PM", "1/2/2006 03:04:05 PM"} {
				if t, err := parse(layout, datestr, loc); err == nil {
					return t, StateDigitSlashWSColonColonAMPM, nil
				}
			}
		}

	case StateWeekdayCommaOffset:
		// Monday, 02 Jan 2006 15:04:05 -0700
		// Monday, 02 Jan 2006 15:04:05 +0100
		t, err := parse("Monday, 02 Jan 2006 15:04:05 -0700", datestr, loc)
		return t, StateWeekdayCommaOffset, err
	case StateWeekdayAbbrevComma: // Starts alpha then comma
		// Mon, 02-Jan-06 15:04:05 MST
		// Mon, 02 Jan 2006 15:04:05 MST
		t, err := parse("Mon, 02 Jan 2006 15:04:05 MST", datestr, loc)
		return t, StateWeekdayAbbrevComma, err
	case StateWeekdayAbbrevCommaOffset:
		// Mon, 02 Jan 2006 15:04:05 -0700
		// Thu, 13 Jul 2017 08:58:40 +0100
		// RFC1123Z    = "Mon, 02 Jan 2006 15:04:05 -0700" // RFC1123 with numeric zone
		t, err := parse("Mon, 02 Jan 2006 15:04:05 -0700", datestr, loc)
		return t, StateWeekdayAbbrevCommaOffset, err
	case StateWeekdayAbbrevCommaOffsetZone:
		// Tue, 11 Jul 2017 16:28:13 +0200 (CEST)
		t, err := parse("Mon, 02 Jan 2006 15:04:05 -0700 (CEST)", datestr, loc)
		return t, StateWeekdayAbbrevCommaOffsetZone, err
	case StateHowLongAgo:
		// 1 minutes ago
		// 1 hours ago
		// 1 days ago
		switch {
		case strings.Contains(datestr, "minutes ago"):
			t, err := agoTime(datestr, time.Minute)
			return t, StateHowLongAgo, err
		case strings.Contains(datestr, "hours ago"):
			t, err := agoTime(datestr, time.Hour)
			return t, StateHowLongAgo, err
		case strings.Contains(datestr, "days ago"):
			t, err := agoTime(datestr, Day)
			return t, StateHowLongAgo, err
		}
	}

	return time.Time{}, StateStart, fmt.Errorf("Could not find date format for %s", datestr)
}

func agoTime(datestr string, d time.Duration) (time.Time, error) {
	dstrs := strings.Split(datestr, " ")
	m, err := strconv.Atoi(dstrs[0])
	if err != nil {
		return time.Time{}, err
	}
	return time.Now().Add(-d * time.Duration(m)), nil
}
