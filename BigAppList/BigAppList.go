// Package bigapplist provides Steam’s massive list of app names and numbers in
// a convenient form, using a local cache.
//
// The Steam web API at “http://api.steampowered.com/ISteamApps/GetAppList/v2/”
// returns the name and numeric ID of all the current Steam apps as multiple
// megabytes of JSON (without any line breaks!). Therefore, this package caches
// the information in per-user storage, using a ‘Simple’ text format which is
// much smaller and easier to parse.
//
// There are two functions to get AppList structs.  Programs which don't need
// up-to-date information can call LatestCached(). To get the big app list as of
// at most n hours ago, use FromCacheOrWeb(n).
//
//
// Size Considerations
//
// This list really is quite large. At one stage during November 2019, the JSON
// from GetAppList had ~87,000 names containing ~3,145,000 characters in
// ~3,188,000 bytes. The longest name, for app # 1009190, was 114 characters and
// 289 bytes long. The JSON form was 4.7MB long; the simple form was 3.3MB.
// Moreover, this list keeps on growing: in March 2019, it was only ~77,000 apps
// and 4.1MB of JSON.
//
//
// The Simple File Format
//
// The Simple format consists of one header line followed by one line per known
// app. The header line gives the URL used and the date and time of the download:
//   # Data from URL as of YYYY-MM-DD HH:MM:SSZ
// where the Z is literal.
//
// The following lines contain (1) the app ID as a decimal number, (2) a tab and
// (3) the app name as written by the %q verb but with the leading and trailing
// '"' characters removed. This means that certain characters in names will be
// represented by backslash escapes; notably ‘"’ will appear as ‘\"’.
//
package BigAppList // import "github.com/c12h/SteamAPI/BigAppList"

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	steamAPI "github.com/c12h/SteamAPI"
)

// This is the URL from which (this version of) this package gets the huge JSON
// list.
const URL = "http://api.steampowered.com/ISteamApps/GetAppList/v2/"

//

const (
	ourDirName          = "BigAppLists"
	cacheFileNameFormat = `BigAppList@%d.txt`
	cacheFileNameRegexp = `^BigAppList@(\d+).txt$`
)

var ourCacheDir = filepath.Join(steamAPI.CacheDirPath(), ourDirName)

/*======================= Exported Types and Constants =======================*/

type (
	// Steam apps are identified by positive integers.
	SteamAppID = uint32

	// Type NameAndNumber holds the id number and name of a Steam app.
	NameAndNumber struct {
		Name string
		ID   SteamAppID
	}

	// Type NameNumberList is a slice of NameAndNumber.
	NameNumberList []NameAndNumber

	nnl = NameNumberList // internal abbreviation

	// Type AppList is an in-memory copy of (a version of) the Steam App List.
	AppList struct {
		AsOf     time.Time      // When the list was fetched, roughly
		Count    int            // Length of list(s), excluding nulls at end
		ByAppNum NameNumberList // Sorted by app #
		ByNameMC NameNumberList // Sorted by original ("Mixed Case") name
		ByNameUC NameNumberList // Sorted by uppercased name
	}
)

// The zero value of SteamAppID is never used for a real Steam app.
const NullSteamAppID = SteamAppID(0)

const maxAppID = (1<<31 - 1)

var nullItem = NameAndNumber{}

/*============================== Creating Lists ==============================*/

// Function bigappslist.FromCache() returns the latest version of Steam's app
// list that is present in the cache.
//
// If the cache is empty, then (despite its name) FromCache downloads the
// current version of the list from Steam, caches it and returns it.
//
func FromCache() (*AppList, error) {
	const LongLongAgo = uint32(24 * 365 * 1000) // 1000 years should be enough
	return FromCacheOrWeb(LongLongAgo)
}

var (
	regexpCacheName = regexp.MustCompile(`^SteamAppList@(\d+)\.txt$`)
	formatCacheName = "SteamAppList@%d.txt"
)

// Function bigappslist.FromCacheOrWeb(N) returns the latest version of Steam's
// app list from the cache if it is no more than N hours old. Otherwise, it
// downloads the current version of the list from Steam, caches it and returns
// it.
//
// Programs that absolutely need the current list can call FromCacheOrWeb(0).
// Since each download is ~5MB (and growing), using values such as 1, 24, 3*24
// or even 7*24 might be kinder to some users.
//
func FromCacheOrWeb(maxAgeHours uint32) (*AppList, error) {
	dh, err := os.Open(ourCacheDir)
	if err != nil {
		return nil, &CacheError{
			Action: "open directory", Path: ourCacheDir, BaseError: err}
	}

	entries, err := dh.Readdir(-1)
	if err != nil {
		return nil, &CacheError{
			Action: "read directory", Path: ourCacheDir, BaseError: err}
	}

	var newestFile os.FileInfo
	var newestModTime int64 = 0
	for _, fi := range entries {
		if m := regexpCacheName.FindStringSubmatch(fi.Name()); m != nil {
			modTimeFromName, err := strconv.ParseInt(m[1], 10, 64)
			if err != nil && modTimeFromName > newestModTime {
				newestFile, newestModTime = fi, modTimeFromName
			}
		}
	}
	if newestFile != nil {
		return FromSimpleFile(filepath.Join(ourCacheDir, newestFile.Name()))
	} else {
		return fetchAndCache()
	}
}

func FromSimpleFile(path string) (*AppList, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, &CacheError{
			Action: "open file", Path: ourCacheDir, BaseError: err}
	}
	defer fh.Close()
	return FromSimpleFormat(fh, path, true)
}

func FromJSONFile(path string) (*AppList, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, &CacheError{
			Action: "open file", Path: ourCacheDir, BaseError: err}
	}
	defer fh.Close()
	return FromJSON(fh, path, true)
}

func fetchAndCache() (*AppList, error) {
	resp, err := http.Get(URL)
	if err != nil {
		return nil, &WebError{Action: "GET", URL: URL, BaseError: err}
	}
	defer resp.Body.Close()
	if isHTTPerror(resp.StatusCode) {
		return nil, &WebError{Action: "GET", URL: URL,
			StatusCode: resp.StatusCode, StatusText: resp.Status}
	}

	unixTime := time.Now().Unix()

	al, err := FromJSON(resp.Body, "Steam web API", false)
	if err != nil {
		return nil, err
	}

	newFilePath := filepath.Join(
		ourCacheDir,
		fmt.Sprintf(cacheFileNameFormat, unixTime))
	err = al.WriteFile(newFilePath,
		os.O_CREATE|os.O_WRONLY|os.O_EXCL)
	if err != nil {
		// os.IsExistg(err) ???
		return nil, err
	}

	return al, nil
}

/*========================== Searching the List(s) ===========================*/

// Method FindNameForNumber searches AppList.ByAppNum for an element with ID
// greater than or equal to targetID, using binary search.
//
// If it finds an exact match, this method returns the index of the matching
// element in AppList.ByAppNum and the name from that element.
// Otherwise, if AppList.ByAppNum contains any elements with AppID exceeding
// targetID, FindNameForNumber returns the index of that element and an empty
// string.
// Otherwise, if all of the IDs in AppList are less than targetID, it returns
// (AppList.Count + 1) and an empty string.
//
// AppList.ByAppNum has an extra zero-valued element at the end, so the integer
// return value is always a safe index for AppList.ByAppNum. (In other words,
//	i, name := al.FindNameForNumber(t)
//	nameNumber := al.ByAppNum[i]
// will never cause a bounds error).
//
func (al *AppList) FindNameForNumber(targetID SteamAppID) (int, string) {
	i := sort.Search(al.Count,
		func(j int) bool {
			return al.ByAppNum[j].ID >= targetID
		})
	// If the search fails, al.ByAppNum[i] is the ‘sentinel’ at the end of the slice.
	name := ""
	if al.ByAppNum[i].ID == targetID {
		name = al.ByAppNum[i].Name
	}
	return i, name
}

// Method FindNumberForName does a binary search of AppList.ByNameMC for an
// element with Name greater than or equal to targetName, using Go's usual
// byte-by-byte string comparisons.
//
// If it finds an exact match, this method returns the index of that element of
// AppList.ByNameMC and the ID from that element.
// Otherwise, if AppList.ByNameMC contains any elements with AppID which sort
// after targetName, FindNumberForName returns the index of that element and an
// empty string.
// Otherwise, if all of the names in AppList compare less than targetName, this
// method returns (AppList.Count + 1) and an empty string. In closely-related
// news, AppList.ByNameMC[AppList.Count+1] always exists (and has Name="" and
// ID=NullSteamAppID).
//
func (al *AppList) FindNumberForName(targetName string) (int, SteamAppID) {
	// Is Unicode order good enough here???
	i := sort.Search(al.Count,
		func(j int) bool {
			return al.ByNameMC[j].Name >= targetName
		})
	// If the search fails, al.ByNameMC[i] is the ‘sentinel’ at the end of the slice.
	appID := NullSteamAppID
	if al.ByNameMC[i].Name == targetName {
		appID = al.ByNameMC[i].ID
	}
	return i, appID
}

// Method FindNumberForName does a binary search of AppList.ByNameUC for an
// element with Name greater than or equal to  targetName, using Go's usual
// byte-by-byte string comparisons.
//
// If it finds an exact match, this method returns the index of that element of
// AppList.ByNameUC and the ID from that element.
// Otherwise, if AppList.ByNameUC contains any elements with AppID which sort
// after targetName, FindNumberForName returns the index of that element and an
// empty string.
// Otherwise, if all of the names in AppList compare less than targetName, this
// method returns (AppList.Count + 1) and an empty string. AppList.ByNameUC has
// an extra, zero-valued element at that index.
//
func (al *AppList) FindNumberForNameUC(targetName string) (int, SteamAppID) {
	targetName = strings.ToUpper(targetName)
	// Is Unicode order good enough here???
	i := sort.Search(al.Count,
		func(j int) bool {
			return strings.ToUpper(al.ByNameUC[j].Name) >= targetName
		})
	// If the search fails, al.ByNameUC[i] is the ‘sentinel’ at the end of the slice.
	appID := NullSteamAppID
	if al.ByNameUC[i].Name == targetName {
		appID = al.ByNameUC[i].ID
	}
	return i, appID
}

/*================================== Errors ==================================*/

func logBug(data []byte, prefix, source string, isFile bool,
	format string, args ...interface{}) {

	if isFile {
		source = fmt.Sprintf("file %q", source)
	}
	output := fmt.Sprintf(
		"\n%s (prog %s) %s %s %s\n",
		time.Now().Format("2006-01-02 15:04:05Z"),
		os.Args[0], prefix, source,
		fmt.Sprintf(format, args...))
	if len(data) > 0 {
		output += fmt.Sprintf("  %q\n", data)
	}

	BugsLogPath := filepath.Join(ourCacheDir, "BUGS.log")
	fh, err := os.OpenFile(BugsLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		fh = os.Stderr
		intro := fmt.Sprintf(
			"%s: could not append following to file %q (%s): \n ",
			filepath.Base(os.Args[0]), BugsLogPath)
		output = intro + output[1:]
	}
	fmt.Fprint(fh, output)
	fh.Sync()
	fh.Close()
}

type CacheError struct {
	Action    string
	Path      string
	BaseError error
}

func (e *CacheError) Error() string {
	return fmt.Sprintf("cannot %s %q: %s", e.Action, e.Path, e.BaseError)
}

func (e *CacheError) Unwrap() error { return e.BaseError }

//

func isHTTPerror(code int) bool {
	return code/100 != 2
}

type WebError struct {
	Action     string
	URL        string
	StatusCode int
	StatusText string
	BaseError  error
}

func (e *WebError) Error() string {
	if e.BaseError != nil {
		return fmt.Sprintf("cannot %s %q: %s",
			e.Action, e.URL, e.BaseError)
	} else {
		return fmt.Sprintf("cannot %s %q: HTTP status %d (%s)",
			e.BaseError, e.URL, e.StatusCode, e.StatusText)
	}
}

func (e *WebError) Unwrap() error { return e.BaseError }
