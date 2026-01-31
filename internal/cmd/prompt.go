package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// PromptSelect displays numbered options and returns the selected index
// Returns -1 if cancelled (user enters "0" or empty)
func PromptSelect(message string, options []string) int {
	if len(options) == 0 {
		return -1
	}

	fmt.Println()
	fmt.Println(message)
	for i, opt := range options {
		fmt.Printf("  [%d] %s\n", i+1, opt)
	}
	fmt.Printf("  [0] Skip\n")
	fmt.Println()
	fmt.Print("? Select: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return -1
	}

	input = strings.TrimSpace(input)
	if input == "" || input == "0" {
		return -1
	}

	choice, err := strconv.Atoi(input)
	if err != nil || choice < 1 || choice > len(options) {
		return -1
	}

	return choice - 1
}

// IsInteractive returns true if stdin is a terminal and --yes flag is not set
func IsInteractive() bool {
	if IsYesMode() {
		return false
	}
	return term.IsTerminal(int(os.Stdin.Fd()))
}
