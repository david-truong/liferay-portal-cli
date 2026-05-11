package portal

import (
	"os/user"
	"strings"
)

// SafeUsername returns the current user's name with any path-separator
// characters stripped. On Windows AD systems user.Current() can return a
// name like `DOMAIN\user`, which becomes a directory when interpolated
// into a filename such as `app.server.<user>.properties`. Stripping
// everything up to the last separator keeps the local-account part.
//
// On Unix where usernames never contain '\' or '/', SafeUsername is a
// no-op pass-through of user.Current().Username.
func SafeUsername() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	return sanitizeUsername(u.Username), nil
}

func sanitizeUsername(name string) string {
	if i := strings.LastIndexAny(name, `\/`); i >= 0 {
		name = name[i+1:]
	}
	return name
}
