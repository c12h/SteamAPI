package BigAppList

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

/*============================ Loading the Lists =============================*/

func FromJSON(r io.Reader, source string, isFile bool) (*AppList, error) {
	const (
		formatStart   = `{"applist":{"apps":[{"appid":%d,"name":%q}`
		safePeekStart = len(`{"applist":{"apps":[{"appid":1,"name":"`)
		formatLater   = `,{"appid":%d,"name":%q}`
		safePeekLater = len(`,{"appid":1,"name":"}`)
	)

	al := new(AppList)

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
		return nil, &ParseError{
			AtStart: true, Source: source, IsFile: isFile, Excerpt: s}
	} else if err != nil {
		return nil, &ReadError{Source: source, IsFile: isFile,
			AtStart: true, BaseError: err}
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
			return nil, &ParseError{
				Source: source, IsFile: isFile, Excerpt: s}
		} else if err != nil {
			return nil, &ReadError{Source: source, IsFile: isFile,
				BaseError: err}
		} else {
			maybeInsert(number, name, al, source, isFile)
		}

	}

	return finishAppList(al, time.Now().Unix())
}

func peek(bufReader *bufio.Reader, limit int) []byte {
	peek, _ := bufReader.Peek(limit)
	ret := []byte{0}
	ret = append(ret, peek...)
	ret = ret[1:]
	return ret
}

func FromSimpleFormat(r io.Reader, source string, isFile bool) (*AppList, error) {
	al := new(AppList)

	s := bufio.NewScanner(r)

	var unixTime int64
	if !s.Scan() {
		return nil, &ReadError{IsEmpty: true, Source: source, IsFile: isFile}
	}
	// parse date+time from first line

	nameBuf := make([]byte, 0, 10) //#D#: later: 512
	for s.Scan() {
		line := s.Bytes()
		i, number := 0, int64(0)
		name := ""
		for i < len(line) && line[i] >= '0' && line[i] <= '9' {
			number *= 10
			number += int64(line[i] - '0')
		}
		if i > 0 || i < len(line) || line[i] == '\t' {
			if cap(nameBuf) < len(line) {
				nameBuf = make([]byte, 1, 2*len(line))
			} else {
				nameBuf = nameBuf[:1]
			}
			nameBuf[0] = '"'
			nameBuf = append(nameBuf, line[i+1:]...)
			nameBuf = append(nameBuf, '"')
			n, err := fmt.Fscanf(bytes.NewReader(nameBuf), "%q", &name)
			if n < 1 || err != nil {
				name = ""
			}
		}
		if number <= 0 || number > maxAppID || name == "" {
			return nil, &ParseError{
				Excerpt: line, Source: source, IsFile: isFile}
		}
		maybeInsert(number, name, al, source, isFile)
	}

	return finishAppList(al, unixTime)
}

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

func finishAppList(al *AppList, unixTime int64) (*AppList, error) {
	al.Count = len(al.ByAppNum)
	al.AsOf = time.Unix(unixTime, 0)

	sort.Sort(listByAppNum(al.ByAppNum))
	sort.Sort(listByAppNum(al.ByNameMC))
	sort.Sort(listByAppNum(al.ByNameUC))

	// Append an empty ‘sentinel’ item to each list.
	// (This makes things simpler for the FindXForY methods.)
	al.ByAppNum = append(al.ByAppNum, nullItem)
	al.ByNameMC = append(al.ByNameMC, nullItem)
	al.ByNameUC = append(al.ByNameUC, nullItem)

	return al, nil
}

/*============================ Sorting the Lists =============================*/

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

type ReadError struct {
	Source    string
	IsFile    bool
	AtStart   bool
	IsEmpty   bool
	BaseError error
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

type ParseError struct {
	Source  string
	IsFile  bool
	AtStart bool
	Excerpt []byte
}

func (e *ParseError) Error() string {
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
