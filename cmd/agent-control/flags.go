package main

import (
	"flag"
	"strings"
)

type boolFlag interface {
	IsBoolFlag() bool
}

func parseFlexible(fs *flag.FlagSet, args []string) error {
	return fs.Parse(reorderArgs(fs, args))
}

func reorderArgs(fs *flag.FlagSet, args []string) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		flags = append(flags, arg)
		if strings.Contains(arg, "=") {
			continue
		}
		name := strings.TrimLeft(arg, "-")
		flagDef := fs.Lookup(name)
		if flagDef == nil {
			continue
		}
		if bf, ok := flagDef.Value.(boolFlag); ok && bf.IsBoolFlag() {
			continue
		}
		if i+1 < len(args) {
			next := args[i+1]
			if next != "--" {
				flags = append(flags, next)
				i++
			}
		}
	}
	return append(flags, positionals...)
}
