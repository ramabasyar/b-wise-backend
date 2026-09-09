// Package assets — embedded fonts (Roboto, Apache 2.0) utk export PDF.
package assets

import _ "embed"

//go:embed fonts/Roboto-Regular.ttf
var RobotoRegular []byte

//go:embed fonts/Roboto-Bold.ttf
var RobotoBold []byte
