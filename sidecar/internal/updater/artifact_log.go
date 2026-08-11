package updater

import (
	"fmt"
	"io"
	"log"
)

func writeArchiveLog(logW io.Writer, format string, args ...any) {
	if logW != nil {
		fmt.Fprintf(logW, format+"\n", args...)
		return
	}
	log.Printf("archive: "+format, args...)
}
