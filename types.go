package SteamAPI

import (
	"fmt"
	"os"
	"path/filepath"
)

// Type SteamItemID holds an 'app id' (a positive integer) denoting a Steam App
// (per https://partner.steamgames.com/doc/store/application).
//
// It is named "SteamItemID" instead of "SteamAppId" to allow for a possible
// (but NOT likely) future change to also denote Steam ‘bundles’ (AKA ‘subs’;
// see https://partner.steamgames.com/doc/store/application/bundles) and/or
// Steam packages (https://partner.steamgames.com/doc/store/application/packages).
//
type SteamItemID uint32

// NullSteamID is the zero value for a SteamItemID.
const NullSteamID = SteamItemID(0)

/*=============================== Directories ================================*/

// FIXME: should use basedirs here, once I write it.

// ??? is panicking on MkdirAll() failure OK ???

const baseDirsRelPath = "SteamAPI"

var moduleConfigDir, moduleCacheDir string

func ConfigDirPath() string {
	if moduleConfigDir == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			panic("os.UserConfigDir() failed: " + err.Error())
		}
		moduleConfigDir = filepath.Join(dir, baseDirsRelPath)
		EnsureDirExists(moduleConfigDir)
	}
	return moduleConfigDir
}

func CacheDirPath() string {
	if moduleCacheDir == "" {
		dir, err := os.UserCacheDir()
		if err != nil {
			panic("os.UserCacheDir() failed: " + err.Error())
		}
		moduleCacheDir = filepath.Join(dir, baseDirsRelPath)
		EnsureDirExists(moduleCacheDir)
	}
	return moduleCacheDir
}

func EnsureDirExists(path string) {
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		err = os.MkdirAll(path, 0o744)
	}
	// ???XXX Is panic() good enough here?
	if err != nil || (fi != nil && !fi.IsDir()) {
		panic(fmt.Sprintf("SteamAPI needs directory at %q", path))
	}
}
