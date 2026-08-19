package gicisky

import "tinygo.org/x/bluetooth"

// ManufacturerCompanyID is the company identifier Gicisky tags advertise under.
// 0x5053 is ASCII "PS", for PICKSMART.
const ManufacturerCompanyID = 0x5053

// advertisementLength is the payload this parser understands, and the length
// is matched exactly rather than as a minimum. A different length is a format
// this code has not seen, and reading its first five bytes as if it were this
// one would yield a confident, wrong panel size.
const advertisementLength = 5

// Advertisement is what a tag says about itself without being connected to.
// This is the only identity worth trusting: the advertised local name is
// absent from some packets entirely, and carries the MAC rather than the model
// when it is present.
type Advertisement struct {
	// Hardware is the raw 16-bit value. Its top two bits vary between panel
	// families and are not understood here, which is why ID masks them off.
	Hardware uint16
	ID       uint16
	Firmware uint16
	// Battery is the raw byte; Voltage converts it. No charge percentage is
	// derived from it, because the voltage curve of a coin cell is not linear
	// and a made-up percentage reads as fact.
	Battery uint8
}

func (a Advertisement) Voltage() float64 { return float64(a.Battery) / 10 }

// ParseAdvertisement decodes the manufacturer data of company 0x5053.
//
// The byte order is not a mistake. The id's low byte comes first and its high
// byte last, with the firmware version between them:
//
//	byte  0     1       2   3       4
//	      id.lo battery firmware    id.hi
func ParseAdvertisement(data []byte) (Advertisement, bool) {
	if len(data) != advertisementLength {
		return Advertisement{}, false
	}
	hardware := uint16(data[4])<<8 | uint16(data[0])
	return Advertisement{
		Hardware: hardware,
		ID:       hardware & 0x3FFF,
		Firmware: uint16(data[2])<<8 | uint16(data[3]),
		Battery:  data[1],
	}, true
}

func giciskyAdvertisement(result bluetooth.ScanResult) (Advertisement, bool) {
	for _, element := range result.ManufacturerData() {
		if element.CompanyID != ManufacturerCompanyID {
			continue
		}
		return ParseAdvertisement(element.Data)
	}
	return Advertisement{}, false
}
