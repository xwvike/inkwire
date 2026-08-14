package nrfepd

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Mode is what the tag draws when nobody is sending it pages.
//
// The tag has a clock of its own and the firmware can draw a calendar or a
// clock face from it, waking on its own to redraw. That is the firmware's
// drawing, not this program's: there is nothing here to render it from and no
// preview of it to show.
type Mode uint8

const (
	// ModePicture leaves the panel showing whatever was last sent to it.
	ModePicture Mode = 0
	// ModeCalendar redraws once a day, at midnight.
	ModeCalendar Mode = 1
	// ModeClock redraws every minute.
	ModeClock Mode = 2
)

func (m Mode) String() string {
	switch m {
	case ModePicture:
		return "picture"
	case ModeCalendar:
		return "calendar"
	case ModeClock:
		return "clock"
	}
	return fmt.Sprintf("Mode(%d)", uint8(m))
}

// ParseMode reads a mode by name.
func ParseMode(name string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "picture":
		return ModePicture, nil
	case "calendar":
		return ModeCalendar, nil
	case "clock":
		return ModeClock, nil
	}
	return 0, fmt.Errorf("unknown mode %q: use picture, calendar or clock", name)
}

// cmdSetTime carries the time and the mode together, because the firmware sets
// both from one message and redraws immediately afterwards.
const cmdSetTime = 0x20

// weekStart values, which decide which column a calendar starts on.
const cmdSetWeekStart = 0x21

// SetTimeFrame builds the command that sets the tag's clock and what it draws
// from it.
//
// The wire format is a four byte timestamp, a timezone in whole hours, and the
// mode. The firmware adds the timezone to the timestamp and keeps the sum,
// which is the whole use it makes of that byte: nothing stores or reports it
// afterwards.
//
// So the offset is folded in here and the timezone byte is sent as zero. It is
// the same arithmetic and it is exact for the zones the byte cannot describe —
// India is five and a half hours ahead, Nepal five and three quarters, and a
// field counting whole hours puts both of them on the wrong minute forever.
func SetTimeFrame(when time.Time, mode Mode) []byte {
	_, offset := when.Zone()
	local := when.Unix() + int64(offset)
	return []byte{
		cmdSetTime,
		byte(local >> 24), byte(local >> 16), byte(local >> 8), byte(local),
		0,
		byte(mode),
	}
}

// WeekStartFrame builds the command that says which day a calendar week starts
// on: 0 is Sunday, 1 is Monday, and the firmware refuses anything above six.
func WeekStartFrame(day time.Weekday) []byte {
	return []byte{cmdSetWeekStart, byte(day)}
}

// ModeSession hands the tag back to its own clock.
//
// It exists because pushing a page takes that clock away. The refresh at the
// end of every page sets the tag to picture mode, so a tag that was keeping
// time stops the first time anything is sent to it, and without this there
// would be no way back short of the vendor's web tool. A program that can only
// remove a device's abilities is not a good citizen on that device.
func ModeSession(ctx context.Context, link transport, when time.Time, mode Mode, weekStart *time.Weekday, timings Timings, logf func(string, ...any)) error {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	if err := link.Write([]byte{cmdInit}); err != nil {
		return fmt.Errorf("initialise: %w", err)
	}
	// The panel is asked what it is for the sake of saying so. Nothing here
	// depends on the answer: the firmware draws this screen itself and knows
	// its own geometry, so an unfamiliar panel is not a reason to refuse.
	config, _, err := listen(ctx, link, timings.response(), logf)
	if err != nil {
		return err
	}
	if model, known := config.Model(); known {
		logf("panel is %s", model)
	} else {
		logf("panel reports model 0x%02x, which is not in this build's table", config.ModelID)
	}

	if weekStart != nil {
		if err := link.Write(WeekStartFrame(*weekStart)); err != nil {
			return fmt.Errorf("set the first day of the week: %w", err)
		}
	}
	// Built last and written immediately, so that the clock is set to the time
	// it is when it arrives rather than the time it was when the command was
	// decided on.
	logf("setting the clock to %s and the mode to %s", when.Format("2006-01-02 15:04:05 -0700"), mode)
	if err := link.Write(SetTimeFrame(when, mode)); err != nil {
		return fmt.Errorf("set the time: %w", err)
	}
	// Setting the time redraws the panel whatever the mode, so this needs the
	// same wait a page does: leaving now would sleep the panel mid-refresh.
	return waitForRefresh(ctx, timings.settle(), logf)
}
