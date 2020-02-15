package dateparse

import (
	"fmt"
	"testing"
)

func TestParseAny(t *testing.T) {
	fmt.Println(ParseAny("2020年02月02日 02:02"))
	fmt.Println(ParseLocal("2020年02月02日 02:02"))
	fmt.Println(ParseLocal("1 days ago"))
	fmt.Println(ParseLocal("1 hours ago"))
	fmt.Println(ParseLocal("1 minutes ago"))
}
