package system

import (
	"bufio"
	"os"
	"strings"
)

type DistroType int

const (
	Unknown DistroType = iota
	Debian
	Ubuntu
	CentOS
)

func (d DistroType) String() string {
	return [...]string{"Unknown", "Debian", "Ubuntu", "CentOS"}[d]
}

func DetectDistro() DistroType {
	if _, err := os.Stat("/etc/debian_version"); err == nil {
		return detectDebianBased()
	}
	if _, err := os.Stat("/etc/redhat-release"); err == nil {
		return CentOS
	}
	return Unknown
}

func detectDebianBased() DistroType {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return Debian
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "Ubuntu") {
			return Ubuntu
		}
	}
	return Debian
}
