package main

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/ajstarks/svgo"
)

func f64s(val float64) string {
	return strconv.FormatFloat(val, 'f', -1, 64)
}

func polarToCartesian(centerX, centerY, radius, angleInDegrees float64) (x, y float64) {
	var angleInRadians = (angleInDegrees - 90) * math.Pi / 180.0
	x = centerX + (radius * math.Cos(angleInRadians))
	y = centerY + (radius * math.Sin(angleInRadians))
	return
}

func segment(x, y, radius, startAngle, endAngle, width float64) string {

	startX, startY := polarToCartesian(x, y, radius, endAngle)
	endX, endY := polarToCartesian(x, y, radius, startAngle)

	parts := []string{
		"M", f64s(startX - width), f64s(startY),
		"L", f64s(startX), f64s(startY),
		"A", f64s(radius), f64s(radius), "0", "0", "0", f64s(endX), f64s(endY),
		"L", f64s(endX), f64s(endY + width),
		"A", f64s(radius - width), f64s(radius - width), "0", "0", "1", f64s(startX - width), f64s(startY),
		"Z",
	}

	return strings.Join(parts, " ")
}

const (
	canvasSize        int     = 500
	startAngle        float64 = 4.0
	endAngle          float64 = 86.0
	outerWidthProp    float64 = 0.14
	innerWidthProp    float64 = 0.12
	innerOuterGapProp float64 = 0.04
)

func main() {

	centerX := float64(canvasSize / 2)
	centerY := float64(canvasSize / 2)

	outerRadius := float64(canvasSize) / 2
	outerWidth := outerWidthProp * float64(canvasSize)
	outerFill := "fill:#e74c3c;"

	innerRadius := float64(canvasSize)/2 - outerWidth - innerOuterGapProp*float64(canvasSize)
	innerWidth := innerWidthProp * float64(canvasSize)
	innerFill := "fill:#2980b9;"

	rotate := func(angle int) string {
		return fmt.Sprintf("transform=\"rotate(%d %d %d)\"", angle, int(centerX), int(centerY))
	}

	canvas := svg.New(os.Stdout)
	canvas.Start(canvasSize, canvasSize)

	canvas.Path(segment(centerX, centerY, outerRadius, startAngle, endAngle, outerWidth), outerFill)
	canvas.Path(segment(centerX, centerY, outerRadius, startAngle, endAngle, outerWidth), rotate(-90), outerFill)
	canvas.Path(segment(centerX, centerY, outerRadius, startAngle, endAngle, outerWidth), rotate(-180), outerFill)
	canvas.Path(segment(centerX, centerY, outerRadius, startAngle, endAngle, outerWidth), rotate(-270), outerFill)

	canvas.Path(segment(centerX, centerY, innerRadius, startAngle, endAngle, innerWidth), rotate(-45), innerFill)
	canvas.Path(segment(centerX, centerY, innerRadius, startAngle, endAngle, innerWidth), rotate(-135), innerFill)
	canvas.Path(segment(centerX, centerY, innerRadius, startAngle, endAngle, innerWidth), rotate(-225), innerFill)

	canvas.End()
}
