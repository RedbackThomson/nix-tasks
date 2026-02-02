package ui

import "fmt"

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorGray   = "\033[90m"
)

// Green returns text in green
func Green(s string) string {
	return fmt.Sprintf("%s%s%s", colorGreen, s, colorReset)
}

// Red returns text in red
func Red(s string) string {
	return fmt.Sprintf("%s%s%s", colorRed, s, colorReset)
}

// Yellow returns text in yellow
func Yellow(s string) string {
	return fmt.Sprintf("%s%s%s", colorYellow, s, colorReset)
}

// Blue returns text in blue
func Blue(s string) string {
	return fmt.Sprintf("%s%s%s", colorBlue, s, colorReset)
}

// Gray returns text in gray
func Gray(s string) string {
	return fmt.Sprintf("%s%s%s", colorGray, s, colorReset)
}
