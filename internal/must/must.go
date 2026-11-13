// Package must holds what a package makes once, at start-up, from its own
// declarations: a value that can only fail if the package itself is
// wrong, which its tests then find.
package must

// Be is the value made, and panics with the error when it was not made.
// Use it only where the error says the package's own code is wrong, never
// for anything a caller or a file gives.
func Be[Made any](made Made, err error) Made {
	if err != nil {
		panic(err)
	}
	return made
}
