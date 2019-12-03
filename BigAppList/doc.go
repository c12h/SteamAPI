// Package BigAppList provides Steam’s massive list of app names and numbers in
// a convenient form, using a local cache.
//
// The Steam web API at “http://api.steampowered.com/ISteamApps/GetAppList/v2/”
// returns the name and numeric ID of all the current Steam apps as multiple
// megabytes of JSON (without any line breaks!). Therefore, this package caches
// the information in per-user storage, using a ‘Terse’ text format which is
// much smaller and easier to parse.
//
// There are two functions to get AppList structs.  Programs which don't need
// up-to-date information can call LatestCached(). To get the big app list as of
// at most n hours ago, use FromCacheOrWeb(n).
//
// Due to the widespread confusion between Microsoft’s ‘Western’ character set
// (CP1252) and Unicode, some names contain byte values 0x92 and 0x95, which
// mean "’" (U+2019 and "™" (U+2122) respectively in CP1252. This module translates
// them to proper UTF-8.
//	WHAT ABOUT \t then? See app 1089230.
//
//
// Size Considerations
//
// This list really is quite large. At one stage during November 2019, the JSON
// from GetAppList had ~87,000 names containing ~3,145,000 characters in
// ~3,188,000 bytes. The longest name, for app # 1009190, was 114 characters and
// 289 bytes long. The JSON form was 4.7MB long; the terse form was 3.3MB.
// Moreover, this list keeps on growing: in March 2019, it was only ~77,000 apps
// and 4.1MB of JSON.
//
//
// The Terse File Format
//
// The Terse format consists of one header line followed by one line per known
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
