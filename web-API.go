package SteamAPI

// Package steamAPI provides convenient access to Steam's Web API.
//
// Steam's Web API
//
// Valve provide a documented, supported, public web API at http://api.steampowered.com
// (and https://api.steampowered.com). Applications make HTTP GET requests to URLs like
//	http://api.steampowered.com/ISteamWebAPIUtil/GetSupportedAPIList/v1/
// which will return JSON text describing all the current APIs. The general format is
//	http://api.steampowered.com/$INTERFACE/$METHOD/v$VERSION/?$P1=$V1&$P2=$V2
// in which $INTERFACE names a group of related methods and each method can have multiple
// versions.
//
// Some methods do not require any authorization, but most require you to specify a user
// key which is tied to your Steam account. (Keys consist of 32 hex digits.) Some methods
// are only available to game publishers etc; they require a special publisher key.
//
// Note that getting a user key requires you to agree to the Steam Web API Terms of Use
// (https://steamcommunity.com/dev/apiterms), which includes a limit of 100,000 calls per
// day to the Web API.
//
// For details, start browsing at https://partner.steamgames.com/doc/webapi_overview and
// https://developer.valvesoftware.com/wiki/Steam_Web_API.
//
// Numeric IDs
//
// Every Steam user has a permanent SteamID, which is a unique (and large) number. Users
// also have a profile name, but that need not be unique and is easily changed. Users can
// opt to have a "vanity URL" ...???
//
// Steam also use numeric entities for various other entities. For example, each App has
// an "AppID" (and "https://store.steampowered.com/app/$AppID" will take you to its Store
// page). Nonetheless, "SteamID" always refers to a user's numeric ID.

// the .../steamAPI-c12h/API-access-key.txt could not be
// used.  configuration file $USER_CONFIG/ (where $USER_CONFIG is the
//

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

/*=========================== HTTP/HTTPS Requests ============================*/

// only errors come from GetAPIkey ...???
// panics if odd # of params
func URLforAPI(iface, method string, version int, flags int, params ...string) (
	string, error,
) {
	if len(params)%2 != 0 {
		panic(fmt.Errorf("Bad call to steamAPI.URLforAPI(): "+
			"no value for parameter %q", params[len(params)-1]))
	}
	var err error
	buf := new(strings.Builder)
	if flags&UseHTTPS != 0 {
		fmt.Fprintf(buf, "https")
	} else {
		fmt.Fprintf(buf, "http")
	}
	fmt.Fprintf(buf, "http://api.steampowered.com/%s/%s/v%d/",
		iface, method, version)
	sep := '?'
	if flags&useKey != 0 {
		sep = '&'
		ak, err := GetAPIkey()
		if err != nil {
			return "", err
		}
		fmt.Fprintf(buf, "?key=%s", ak)
	}
	for i := 0; err == nil && i < len(params); i += 2 {
		fmt.Fprintf(buf, "%c%s=%s", sep, params[i], params[i+1])
		sep = '&'
	}
	return buf.String(), nil
}

const (
	// Use https://... instead of http://...
	UseHTTPS = 1
	// Pass the Access Key in the request parameters (requires https)
	UseKey = 3
	useKey = 2
)

func GetResponse(
	what, who string,
	iface, method string,
	version int,
	flags int,
	params ...string,
) (*http.Response, error) {
	url, err := URLforAPI(iface, method, version, flags, params...)
	if err != nil {
		return nil, err
	}
	response, err := http.Get(url)
	if err != nil {
		if response != nil {
			response.Body.Close()
		}
		return nil, &WebError{Action: "get",
			What: what, Who: who, URL: url, BaseError: err}
	}
	return response, nil
}

func GetJSON(outvar interface{},
	what, who string,
	iface, method string,
	version int,
	flags int,
	params ...string,
) error {
	response, err := GetResponse(what, who, iface, method, version, flags, params...)
	url, err := URLforAPI(iface, method, version, flags, params...)
	if err != nil {
		return err
	}
	if err != nil {
		if response != nil {
			response.Body.Close()
		}
		return &WebError{Action: "get",
			What: what, Who: who, URL: url, BaseError: err}
	}
	defer response.Body.Close()
	//
	d := json.NewDecoder(response.Body)
	err = d.Decode(outvar)
	if err != nil {
		return &WebError{Action: "decode",
			What: what, Who: who, URL: url, BaseError: err}
	}
	return nil
}

/*============================ Utility Functions =============================*/

type WebError struct {
	Action    string
	What      string
	Who       string
	BaseError error
	URL       string
}


func (e *WebError) Unwrap() error { return e.BaseError }

func (e *WebError) Error() string {
	source := e.URL
	if e.What != "" {
		source := e.What
		if e.Who != "" {
			source += " for " + e.Who
		}
	}
	return fmt.Sprintf("cannot %s %s: %s", e.Action, source, e.BaseError)
}
