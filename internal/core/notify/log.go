package notify

import (
	"fmt"
	"os"
)

func debugf(format string, args ...interface{}) {
	if os.Getenv("SSHIELD_DEBUG") == "" {
		return
	}
	fmt.Printf("[sshield-debug] "+format+"\n", args...)
}
