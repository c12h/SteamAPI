package BigAppList

import (
	"bufio"
	"fmt"
	"io"
	"os"
)

func (al *AppList) WriteSimple(w io.Writer, destDesc string, isFile bool) error {
	bufWriter := bufio.NewWriter(w)
	defer bufWriter.Flush()

	heading := `"From ` + URL +
		al.AsOf.Format(`as of 2006-01-02 15:04:05Z"`+"\n")
	_, err := fmt.Fprintf(bufWriter, heading)

	for i := 0; err == nil && i < al.Count; i++ {
		name := fmt.Sprintf("%q", al.ByAppNum[i].Name)
		fmt.Fprintf(bufWriter, "%d\t%s\n",
			al.ByAppNum[i].ID, name[1:len(name)-1])
	}

	if err != nil {
		return &WriteError{Action: "write to",
			Dest: destDesc, IsFile: isFile, BaseError: err}
	}
	return nil
}

func (al *AppList) WriteFile(path string, mode int) error {
	fh, err := os.OpenFile(path, mode, 0o666)
	if err != nil {
		return &WriteError{Action: "create",
			Dest: path, IsFile: true, BaseError: err}
	}

	err = al.WriteSimple(fh, path, true)
	if err != nil {
		return err
	}

	err = fh.Sync()
	if err != nil {
		return &WriteError{Action: "finish writing",
			Dest: path, IsFile: true, BaseError: err}
	}
	err = fh.Close()
	if err != nil {
		return &WriteError{Action: "close new",
			Dest: path, IsFile: true, BaseError: err}
	}

	return nil
}

/*================================== Errors ==================================*/

type WriteError struct {
	Action    string // What we were trying to do
	Dest      string // What we were trying to do that to
	IsFile    bool   // Whether it is a file (in which case Dest is the path)
	BaseError error  // What happened
}

func (e *WriteError) Error() string {
	dest := e.Dest
	if e.IsFile {
		dest = fmt.Sprintf("file %q", e.Dest)
	}
	return fmt.Sprintf("cannot %s %s: %s", e.Action, dest, e.BaseError)
}

func (e *WriteError) Unwrap() error { return e.BaseError }
