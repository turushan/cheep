package main

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const forbidden = rune(0x2014)

func main() {
	root := "."
	if len(os.Args) == 2 {
		root = os.Args[1]
	} else if len(os.Args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: textcheck [root]")
		os.Exit(2)
	}

	failed := false
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "bin" || entry.Name() == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !textFile(path) {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(file)
		line := 0
		for scanner.Scan() {
			line++
			if strings.ContainsRune(scanner.Text(), forbidden) {
				fmt.Fprintf(os.Stderr, "%s:%d contains a forbidden em dash character\n", path, line)
				failed = true
			}
		}
		scanErr := scanner.Err()
		closeErr := file.Close()
		if scanErr != nil {
			return scanErr
		}
		return closeErr
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "text check failed: %v\n", err)
		os.Exit(1)
	}
	if failed {
		os.Exit(1)
	}
}

func textFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".md", ".txt", ".toml", ".yaml", ".yml", ".json", ".sh":
		return true
	default:
		return filepath.Base(path) == "Makefile"
	}
}
