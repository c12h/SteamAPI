package SteamAPI

//???FIXME: should use github.com/c12h/basedirs
//
// This file provides routines to get a "Steam API Key" (see doc.go) from a
// configuration file.
// On Linux, that file will be $XDG_
// file. The user or system manager must create that file in an appropriate
// directory:
//	on Linux, this is usually $HOME/.cache/, unless XDG_

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	configFileDir  = "SteamAPI"
	configFileName = configFileDir + "/steam_api_key.txt"
)

var steamAPIkey = ""

// Function GetAPIkey returns the Steam API key as a string, or returns an
// error.
// If a call succeeds, GetAPIkey records the key and simply returns the
// recorded value on any future calls.  To force a re-read of the file,
// call ReloadAPIkey.
func GetAPIkey() (string, error) {
	if steamAPIkey == "" {
		return readAPIkey() // sets steamAPIkey or returns non-nil
	}
	return steamAPIkey, nil
}

// Function MustGetAPIkey returns the Steam API key as a string, first getting it
// from the configuration file if necessary, or panics if anything goes wrong
// (notably, the configuration file not existing or having a syntax error).
func MustGetAPIkey() string {
	if steamAPIkey == "" {
		var err error
		steamAPIkey, err = readAPIkey() // sets steamAPIkey or returns non-nil
		if err != nil {
			panic(err)
		}
	}
	return steamAPIkey
}

// Function ReloadAPIkey re-reads the configuration file, so that future calls
// to GetAPIkey will return the (possibly) new key.
// If ReloadAPIkey returns non-nil, the old key is retained.
func ReloadAPIkey() error {
	_, err := readAPIkey()
	return err
}

// Function readAPIkey does the real work for this package.
func readAPIkey() (string, error) {
	path, err := os.UserConfigDir()
	if err != nil {
		return "", cannotGSK("find config dir to get Steam API key", "", "", err)
	}
	path = filepath.Join(path, configFileName)
	contents, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", cannotGSK("find", path, "", nil)
		}
		return "", cannotGSK("read", path, "", err)
	}

	newKey := ""
	lineScanner := bufio.NewScanner(bytes.NewBuffer(contents))
	ln := 0
	for lineScanner.Scan() {
		ln++
		text := strings.TrimSpace(lineScanner.Text())
		if text != "" && text[0] != '#' {
			if !steamKeyRegexp.MatchString(text) {
				return "", cannotParse(path, ln, "not a valid")
			} else if newKey == "" {
				newKey = text
			} else if text != newKey {
				return "", cannotParse(path, ln, "more than one")
			}
		}
	}
	err = lineScanner.Err()
	if err != nil {
		return "", cannotGSK("scan", path, "", err)
	} else if newKey == "" {
		return "", cannotGSK("see a Steam API key in", path, "", nil)
	}
	steamAPIkey = newKey
	return newKey, nil

}

/*
This allows whitespace and comments in the config file.

An easier-to-code alternative is to require file to match /^\s*[0-9A-Z]{32}\s*$/
	contents = bytes.TrimSpace(contents)
	if steam_key_regexp.Match(contents) {
		steamAPIkey = string(contents)
	} else {
		return "", cannotGSK(nil, "parse %q", path)
	}
*/

var steamKeyRegexp = regexp.MustCompile(`^[0-9A-Z]{32}$`)

/*---------------------------------- Errors ----------------------------------*/

func cannotParse(path string, lineNum int, problem string) error {
	detail := fmt.Sprintf("at line %d: %s Steam API key", lineNum, problem)
	return &SteamKeyError{
		"parse", path, detail, nil}
}

func cannotGSK(verb, path, details string, baseErr error) error {
	if pathErr, isPathErr := baseErr.(*os.PathError); isPathErr {
		baseErr = pathErr.Unwrap()
	}
	return &SteamKeyError{
		verb, path, details, baseErr}
}

// A SteamKeyError represents a failure to get a Steam API key.
//
type SteamKeyError struct {
	// What we could not do ("read", "parse", "see a Steam API key in", etc):
	Action string
	// The file we could not do it to/with, or "" if irrelevant
	Path string
	// Optional extra information (eg., line number, problem)
	Details string
	// The lower-level error that caused the failure, or nil.
	BaseError error
}

func (e *SteamKeyError) Unwrap() error { return e.BaseError }

func (e *SteamKeyError) Error() string {
	text := ""
	if e.Path != "" {
		text = fmt.Sprintf("file %q%s", e.Path)
	}
	text = fmt.Sprintf("cannot %s %s%s", e.Action, text, e.Details)
	if e.BaseError != nil {
		text += ": " + e.BaseError.Error()
	}
	return text
}
