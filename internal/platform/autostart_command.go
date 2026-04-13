package platform

import (
	"fmt"
	"strings"
)

const AutostartFlag = "--autostart"

func IsAutostartLaunch(args []string) bool {
	for _, arg := range args {
		if arg == AutostartFlag {
			return true
		}
	}
	return false
}

func autostartCommand(exe string) string {
	return quoteWindowsArg(exe) + " " + AutostartFlag
}

func legacyAutostartCommand(exe string) string {
	return fmt.Sprintf("%q", exe)
}

func quoteWindowsArg(arg string) string {
	if arg == "" {
		return `""`
	}
	if !strings.ContainsAny(arg, " \t\n\v\"") {
		return arg
	}

	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, r := range arg {
		switch r {
		case '\\':
			backslashes++
		case '"':
			b.WriteString(strings.Repeat(`\`, backslashes*2+1))
			b.WriteRune(r)
			backslashes = 0
		default:
			if backslashes > 0 {
				b.WriteString(strings.Repeat(`\`, backslashes))
				backslashes = 0
			}
			b.WriteRune(r)
		}
	}
	if backslashes > 0 {
		b.WriteString(strings.Repeat(`\`, backslashes*2))
	}
	b.WriteByte('"')
	return b.String()
}
