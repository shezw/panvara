/*
   Panvara
   internal/interfaces/httpapi/etag.go    2026-07-14
    ______     __  __     ______     ______     __     __
   /\  ___\   /\ \_\ \   /\  ___\   /\___  \   /\ \  _ \ \
   \ \___  \  \ \  __ \  \ \  __\   \/_/  /__  \ \ \/ ".\ \
    \/\_____\  \ \_\ \_\  \ \_____\   /\_____\  \ \__/".~\_\
     \/_____/   \/_/\/_/   \/_____/   \/_____/   \/_/   \/_/.com

   @link    : https://github.com/shezw/panvara
   @author  : shezw
   @email   : hello@shezw.com
*/

package httpapi

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func formatVersionETag(version uint64) string {
	return `"` + strconv.FormatUint(version, 10) + `"`
}

func formatArtifactETag(artifact []byte) string {
	digest := sha256.Sum256(artifact)
	return fmt.Sprintf(`"sha256:%x"`, digest[:])
}

func parseIfMatch(request *http.Request) (uint64, *requestProblem) {
	values := request.Header.Values("If-Match")
	if len(values) == 0 {
		return 0, &requestProblem{
			status: http.StatusPreconditionRequired, code: "if_match_required",
			message: "If-Match with the current record ETag is required",
		}
	}
	if len(values) != 1 {
		return 0, invalidIfMatch()
	}
	value := strings.TrimSpace(values[0])
	if len(value) < 3 || value[0] != '"' || value[len(value)-1] != '"' || strings.Contains(value[1:len(value)-1], `"`) {
		return 0, invalidIfMatch()
	}
	version, err := strconv.ParseUint(value[1:len(value)-1], 10, 64)
	if err != nil || version == 0 {
		return 0, invalidIfMatch()
	}
	return version, nil
}

func invalidIfMatch() *requestProblem {
	return &requestProblem{
		status: http.StatusBadRequest, code: "invalid_if_match",
		message: fmt.Sprintf("If-Match must be one strong numeric ETag such as %s", formatVersionETag(1)),
	}
}
