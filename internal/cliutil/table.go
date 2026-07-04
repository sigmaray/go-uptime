// Package cliutil provides helper functions for CLI output.
package cliutil

import (
	"io"
	"os"

	"github.com/olekukonko/tablewriter"
)

// PrintTable prints a table to stdout.
func PrintTable(headers []string, rows [][]string) {
	printTableTo(os.Stdout, headers, rows)
}

// PrintTableTo prints a table to the specified writer.
func PrintTableTo(w io.Writer, headers []string, rows [][]string) {
	printTableTo(w, headers, rows)
}

func printTableTo(w io.Writer, headers []string, rows [][]string) {
	table := tablewriter.NewWriter(w)
	table.SetHeader(headers)
	table.SetBorder(true)
	table.SetRowLine(false)
	for _, row := range rows {
		table.Append(row)
	}
	table.Render()
}
