package output

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

func Success(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, "✓ "+format+"\n", args...)
}

func Info(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}

func Warning(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "⚠ "+format+"\n", args...)
}

func Error(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}

func Debug(format string, args ...interface{}) {
	// Debug messages are hidden unless debug flag is set
}

func Print(format string, args ...interface{}) {
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}

// Banner prints lines inside a box, for information that must stand out in
// busy output, such as where the JSON API listens.
func Banner(lines ...string) {
	fmt.Fprint(os.Stdout, FormatBanner(lines...))
}

// FormatBanner draws lines inside a rounded box.
func FormatBanner(lines ...string) string {
	width := 0
	for _, line := range lines {
		width = max(width, utf8.RuneCountInString(line))
	}
	inner := width + 3 // two spaces before the text, one after

	var b strings.Builder
	b.WriteString("╭" + strings.Repeat("─", inner) + "╮\n")
	for _, line := range lines {
		pad := width - utf8.RuneCountInString(line)
		b.WriteString("│  " + line + strings.Repeat(" ", pad) + " │\n")
	}
	b.WriteString("╰" + strings.Repeat("─", inner) + "╯\n")
	return b.String()
}
