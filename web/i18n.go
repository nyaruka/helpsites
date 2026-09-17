package web

import "fmt"

// The site's own strings - the chrome around a workspace's content. English for now; translation can be slotted
// in here without touching the templates.

func tr(s string) string {
	return s
}

func trn(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf(one, n)
	}
	return fmt.Sprintf(many, n)
}
