package BigAppList

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

/*============================= Reading the JSON =============================*/

// FromJSON returns an AppList it creates by parsing JSON text from an io.Reader,
// or an error, but not both.
func FromJSON(r io.Reader, source string, isFile bool) (*AppList, error) {
	const (
		formatStart   = `{"applist":{"apps":[{"appid":%d,"name":%q}`
		safePeekStart = len(`{"applist":{"apps":[{"appid":1,"name":"`)
		formatLater   = `,{"appid":%d,"name":%q}`
		safePeekLater = len(`,{"appid":1,"name":"}`)
	)

	al := new(AppList)
	al.AsOf = time.Now().UTC()
	bufReader := bufio.NewReader(r)
	var number int64
	var name string

	s := peek(bufReader, safePeekStart)
	n, err := fmt.Fscanf(bufReader, formatStart, &number, &name)
	if n < 2 {
		s = append(s, "…"...)
		logBug(s,
			"scanf() of", source, isFile,
			" with format %q → %d, %q\n", formatStart, n, err,
		)
		return nil, &JSONParseError{AtStart: true, Excerpt: s,
			Source: source, IsFile: isFile}
	} else if err != nil {
		return nil, &ReadError{AtStart: true, BaseError: err,
			Source: source, IsFile: isFile}
	} else {
		maybeInsert(number, name, al, source, isFile)
	}
	for {
		s = peek(bufReader, safePeekLater)
		if len(s) == 3 && s[0] == ']' && s[1] == '}' && s[2] == '}' {
			break
		}
		n, err := fmt.Fscanf(bufReader, formatLater, &number, &name)
		if n < 2 {
			logBug(s,
				"scanf() of", source, isFile,
				" with format %q → %d, %q\n", formatLater, n, err)
			return nil, &JSONParseError{Excerpt: s,
				Source: source, IsFile: isFile}
		} else if err != nil {
			return nil, &ReadError{BaseError: err,
				Source: source, IsFile: isFile}
		} else {
			// For defunct app 1089230
			last := len(name) - 1
			if name[last] == '\t' {
				name = name[:last]
			}
			posC2 := strings.IndexByte(name, 0xC2)
			if posC2 >= 0 {
				name = fixCP1252(name, posC2, number, source, isFile)
			}
			maybeInsert(number, name, al, source, isFile)
		}

	}

	finishAppList(al)
	return al, nil
}

func peek(bufReader *bufio.Reader, limit int) []byte {
	peek, _ := bufReader.Peek(limit)
	ret := []byte{0}
	ret = append(ret, peek...)
	ret = ret[1:]
	return ret
}

func fixCP1252(s string, posC2 int, number int64, source string, isFile bool) string {
	newB := make([]byte, 0, len(s)+2)
	for posC2 >= 0 {
		newB = append(newB, s[:posC2]...)
		switch s[posC2+1] {
		case 0x99:
			newB = append(newB, "™"...)
		case 0x92:
			newB = append(newB, "’"...)
		default:
			code := s[posC2+1]
			newB = append(newB, 0xC2, code)
			if code < 0xA0 {
				logBug(nil, "In", source, isFile,
					"name for app %d contains weird char %X",
					number, s[posC2+1])
			}
		}
		s = s[posC2+2:]
		posC2 = strings.IndexByte(s, 0xC2)
	}
	newB = append(newB, s...)
	return string(newB)
}

/*======================== Reading the Terse Format =========================*/

const (
	toEOF       = 0
	unknownTime = 0
)

// FromTerseFile reads a text file containing an AppList in the 'terse format'.
func FromTerseFile(fileSpec string) (*AppList, error) {
	fh, err := os.Open(fileSpec)
	if err != nil {
		return nil, &CacheError{
			Action: "open file", Path: fileSpec, BaseError: err}
	}
	defer fh.Close()
	return FromTerseFormat(fh, toEOF, fileSpec, true)
}

// FromTerseFormat reads the preferred textual form of an AppList from any
// io.Reader.
func FromTerseFormat(r io.Reader, ender byte, source string, isFile bool,
) (*AppList, error) {
	lr := &lineReader{bufReader: bufio.NewReader(r), source: source, isFile: isFile}
	return fromTerseFormat(lr, ender)
}

// Some callers will be reading terse-format app lists from TCP sockets, so we
// cannot use a bufio.Scanner (which "may [advance] arbitrarily far past the
// last token"). Instead, we use a bufio.Reader instead a convenient struct.
//
type lineReader struct {
	bufReader *bufio.Reader
	source    string
	isFile    bool
	seenEOF   bool
	lineNum   int
}

// readLine returns (line, atEOF, error) but only in these combinations:
//	(someBytes, false, nil)		the normal case
//	(nil,       false, nil)		an empty line
//	(someBytes, false, non-nil)	error after reading partial line
//	(nil,       false, non-nil)	read error, no partial line
//	(nil,       true,  ?)		EOF reached.
// It always removes any terminating \n or \r\n from line.
//
func readLine(lr *lineReader) ([]byte, bool, error) {
	if lr.seenEOF {
		return nil, true, nil
	}
	line, err := lr.bufReader.ReadBytes('\n')
	if err == io.EOF {
		if len(line) == 0 {
			return nil, true, nil
		}
		lr.seenEOF = true
		err = nil
	}
	if n := len(line); n > 1 && line[n-1] == '\n' {
		if n > 2 && line[n-2] == '\r' {
			n--
		}
		line = line[:n-1]
	}
	lr.lineNum++
	return line, false, err
}

// fromTerseFormat reads a text stream defining an AppList.
func fromTerseFormat(lr *lineReader, ender byte) (*AppList, error) {
	al := new(AppList)

	line, eof, err := readLine(lr)
	if eof {
		return nil, &ReadError{IsEmpty: true,
			Source: lr.source, IsFile: lr.isFile}
	}
	headerTime, problem := int64(0), ""
	match := regexpHeaderLine.FindSubmatch(line)
	if match == nil {
		problem = "is not like ‘" + formatHeaderLine + `’`
	} else {
		t, err := time.Parse(formatHeaderTime, string(match[1]))
		if err != nil {
			problem = fmt.Sprintf("has bad timestamp %q: %s",
				match[1], err)
		} else {
			headerTime = t.Unix()
		}
	}
	if problem != "" {
		return nil, &TerseFormatError{HeaderProblem: problem,
			LineNum: 1, Line: string(line),
			Source: lr.source, IsFile: lr.isFile}
	}
	al.AsOf = time.Unix(headerTime, 0)

	for {
		line, eof, err = readLine(lr)
		if err != nil {
			// read to EOF / ender ...???XXX
			return nil, &ReadError{Source: lr.source, IsFile: lr.isFile,
				BaseError: err}
		}
		if eof {
			break
		}

		if len(line) == 0 || line[0] == '#' {
			continue
		} else if ender != toEOF && line[0] == ender && len(line) == 1 {
			// read to EOF / ender ...???XXX
			break
		}

		i, number := 0, int64(0)
		name := ""
		for ; i < len(line) && line[i] >= '0' && line[i] <= '9'; i++ {
			number *= 10
			number += int64(line[i] - '0')
		}
		if i > 0 && i < len(line) && line[i] == '\t' {
			line[i] = '"'
			line = append(line, '"') // Goes where CR/LF was.
			n, err := fmt.Fscanf(bytes.NewReader(line[i:]), "%q", &name)
			if n < 1 || err != nil {
				name = ""
			}
		}
		if number <= 0 || number > maxAppID || name == "" {
			return nil, &TerseFormatError{
				Line: string(line), LineNum: lr.lineNum,
				Source: lr.source, IsFile: lr.isFile}
		}
		maybeInsert(number, name, al, lr.source, lr.isFile)
	}

	finishAppList(al)
	return al, nil
}

/*======================== Building the AppList value ========================*/

func maybeInsert(number int64, name string, al *AppList, source string, isFile bool) {
	if number == 0 {
		return
	} else if number < 0 || number > maxAppID {
		logBug([]byte{},
			fmt.Sprintf("ignoring suprising appid %d for %q from",
				number, name),
			source, isFile, "")
		return
	}

	appID := SteamAppID(number)
	al.ByAppNum = append(al.ByAppNum, NameAndNumber{Name: name, ID: appID})
	al.ByNameMC = append(al.ByNameMC, NameAndNumber{Name: name, ID: appID})
	name = strings.ToUpper(name)
	al.ByNameUC = append(al.ByNameUC, NameAndNumber{Name: name, ID: appID})
}

// finishAppList finishes setting up an AppList after reading one from JSON or the
// terse format, notably by sorting the component lists.
func finishAppList(al *AppList) {
	al.Count = len(al.ByAppNum)

	sort.Sort(listByAppNum(al.ByAppNum))
	sort.Sort(listByAppNum(al.ByNameMC))
	sort.Sort(listByAppNum(al.ByNameUC))

	// Append an empty ‘sentinel’ item to each list.
	// (This makes things simpler for the FindXForY methods.)
	al.ByAppNum = append(al.ByAppNum, nullItem)
	al.ByNameMC = append(al.ByNameMC, nullItem)
	al.ByNameUC = append(al.ByNameUC, nullItem)
}

type (
	listByAppNum NameNumberList
	listByNameMC NameNumberList
	listByNameUC NameNumberList
)

func (l listByAppNum) Len() int           { return len(l) }
func (l listByAppNum) Less(i, j int) bool { return l[i].ID < l[j].ID }
func (l listByAppNum) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l listByNameMC) Len() int           { return len(l) }
func (l listByNameMC) Less(i, j int) bool { return l[i].Name < l[j].Name }
func (l listByNameMC) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l listByNameUC) Len() int           { return len(l) }
func (l listByNameUC) Less(i, j int) bool { return l[i].Name < l[j].Name }
func (l listByNameUC) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }

/*================================== Errors ==================================*/

// ReadError represents an I/O error while reading something.
type ReadError struct {
	Source    string // Where the text came from.
	IsFile    bool   // Whether Source is a file path.
	AtStart   bool   // Whether the first read failed.
	IsEmpty   bool   // Whether the file or reader seems to be empty.
	BaseError error  // The wrapped error from the actual io.Reader.
}

func (e *ReadError) Error() string {
	source := e.Source
	if e.IsFile {
		source = fmt.Sprintf("file %q", e.Source)
	}
	if e.IsEmpty {
		source = "empty " + source
	} else if e.AtStart {
		source = "start of " + source
	}
	return fmt.Sprintf("cannot read %s: %s", source, e.BaseError)
}

func (e *ReadError) Unwrap() error { return e.BaseError }

//

// JSONParseError represents a problem parsing the JSON form of a BigAppList.
type JSONParseError struct {
	Source  string
	IsFile  bool
	AtStart bool
	Excerpt []byte
}

func (e *JSONParseError) Error() string {
	source := e.Source
	if e.IsFile {
		source = fmt.Sprintf("file %q", e.Source)
	}

	const ellipsis = "…"
	sample := make([]byte, 0, len(e.Excerpt)+2)
	if e.AtStart {
		source = "start of " + source
	} else {
		copy(sample, ellipsis)
	}
	sample = append(sample, e.Excerpt...)
	sample = append(sample, ellipsis...)

	return fmt.Sprintf("cannot parse %q from %s as JSON from GetAppList",
		source, sample)
}

//

// TerseFormatError represents a problem parsing the terse text form of a BigAppList.
//
// The Line field is the offending line. Our Error() method ignores it, but some
// callers may want to report it along with the Error() string.
//
type TerseFormatError struct {
	Source        string // Where the text came from.
	IsFile        bool   // Whether Source is a file path.
	LineNum       int    // Which line we found problematic (1-origin).
	Line          string // The problematic line itself, for any interested callers.
	HeaderProblem string // If non-empty, what is wrong with the first line.
}

func (e *TerseFormatError) Error() string {
	source := e.Source
	if e.IsFile {
		source = fmt.Sprintf("file %q", e.Source)
	}
	if e.HeaderProblem != "" {
		return fmt.Sprintf("header line from %s %s", source, e.HeaderProblem)
	}
	return fmt.Sprintf("cannot parse line %d from %s: %q", e.LineNum, source, e.Line)
}
