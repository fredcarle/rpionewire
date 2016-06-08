package rpionewire

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// DS1820 is a structure that stores the relevant information of
// a DS1820 one wire temperature sensing device
type DS1820 struct {
	ID         uint64
	Name       string
	DeviceType string
	LastTemp   float64
}

const (
	modelDS18S20 = 0x10
	modelDS18B20 = 0x28
)

var _CrcCheckRegex = regexp.MustCompile(`crc=\w+\s(YES|NO)`)
var _TestSampleRegex = regexp.MustCompile(`.*\st=(\d+)`)

// LoadDevices builds a list of available devices
func LoadDevices() ([]*DS1820, error) {
	names, err := findDevices()
	if err != nil {
		return nil, fmt.Errorf("Error finding one wire devices: %v", err)
	}

	devices := make([]*DS1820, len(names))
	for i := range names {
		devices[i], err = newDS1820(names[i])
		if err != nil {
			return nil, fmt.Errorf("Error opening devices %v: %v", devices[i].Name, err)
		}
	}

	return devices, nil
}

// ReadDevices adds the current temperature read by each devices in
// their respectice struct as LastTemp
func ReadDevices(d []*DS1820) error {
	for _, device := range d {
		dataFile, err := os.OpenFile(fmt.Sprintf("/sys/bus/w1/devices/%v/w1_slave", device.Name), os.O_RDONLY|os.O_SYNC, 0666)
		if err != nil {
			return err
		}
		defer dataFile.Close()

		scanner := bufio.NewScanner(dataFile)

		i := 0
		dataFile.Seek(0, 0)
		for scanner.Scan() {
			if i == 0 {
				if err := scanner.Err(); err != nil {
					return fmt.Errorf("EOF without data from w1")
				}
				line := scanner.Text()
				matches := _CrcCheckRegex.FindStringSubmatch(string(line))
				if len(matches) > 0 && matches[1] != "YES" {
					return fmt.Errorf("CRC mismatch on read")
				}
			} else {
				if err := scanner.Err(); err != nil {
					return fmt.Errorf("EOF without data from w1")
				}
				line := scanner.Text()
				matches := _TestSampleRegex.FindStringSubmatch(string(line))
				if len(matches) > 0 {
					v, err := strconv.ParseInt(matches[1], 10, 64)
					if err != nil {
						return err
					}
					device.LastTemp = float64(v) / 1000
				} else {
					return fmt.Errorf("EOF without data from w1")
				}
			}
			i++

		}

	}
	return nil
}

// findDevices scans through the w1 device directory in order to
// return a list of one wire devices
func findDevices() ([]string, error) {
	cmd := exec.Command("modprobe", "w1_gpio", "&&", "modprobe", "w1_therm")
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	dir, err := os.Open("/sys/bus/w1/devices")
	if err != nil {
		return nil, err
	}

	defer dir.Close()

	// reading all the files in the devices directory
	names, err := dir.Readdirnames(0)
	if err != nil {
		return nil, err
	}

	if len(names) <= 1 {
		err := errors.New("files in /sys/bus/w1/devies: no devices found")
		return nil, err
	}
	devicelist := make([]string, 0, len(names)-1)
	for i := range names {

		// We select all the files except w1_bus_master which is not
		// an actual device
		if !strings.Contains(names[i], "w1_bus_master") {
			devicelist = append(devicelist, names[i])
		}
	}

	return devicelist, nil
}

func newDS1820(name string) (*DS1820, error) {
	device := new(DS1820)
	device.Name = name

	if err := device.getID(); err != nil {
		return nil, err
	}

	return device, nil
}

func (d *DS1820) getID() error {
	fn := fmt.Sprintf("/sys/bus/w1/devices/%v/id", d.Name)
	idFile, err := os.OpenFile(fn, os.O_RDONLY, 0666)
	if err != nil {
		return err
	}
	defer idFile.Close()

	var idFileContent uint64
	err = binary.Read(idFile, binary.LittleEndian, &idFileContent)
	if err != nil {
		return fmt.Errorf("Error decoding %v device id: %v", fn, err)
	}

	devicetype := uint8(idFileContent & 0xff)

	switch devicetype {
	case modelDS18B20:
		d.DeviceType = "DS18B20"
	case modelDS18S20:
		d.DeviceType = "DS18S20"
	default:
		return fmt.Errorf("Error decoding %v device id: Unrecognized one wire family code 0x%x", fn, devicetype)
	}

	d.ID = (idFileContent & 0x00ffffffffffff00) >> 8

	return nil
}
