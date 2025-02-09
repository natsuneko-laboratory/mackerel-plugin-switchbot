package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	mp "github.com/mackerelio/go-mackerel-plugin"
	"github.com/nasa9084/go-switchbot/v4"
)

// --------------------
// instance methods
// --------------------

type SwitchBotPlugin struct {
	Prefix          string
	Targets         []string
	SwitchBotClient *switchbot.Client
	Statuses        map[string]*switchbot.DeviceStatus
}

func (p SwitchBotPlugin) FetchStatuses() error {
	for _, target := range p.Targets {
		status, err := p.SwitchBotClient.Device().Status(context.Background(), target)
		if err != nil {
			return err
		}

		p.Statuses[target] = &status
	}

	return nil
}

func (p SwitchBotPlugin) FetchMetrics() (map[string]float64, error) {
	dict := map[string]float64{}

	for _, target := range p.Targets {
		status, ok := p.Statuses[target]
		if !ok {
			return nil, fmt.Errorf("no status for target %s", target)
		}

		supports := SupportedMetrics[status.Type]

		for _, support := range supports {
			name := fmt.Sprintf("%s.%s", target, support.Name)
			dict[name] = support.ValueFunc(status)
		}
	}

	return dict, nil
}

func (p SwitchBotPlugin) GetPrefix() string {
	if p.Prefix == "" {
		return "switchbot"
	}

	return p.Prefix
}

func (p SwitchBotPlugin) GraphDefinition() map[string]mp.Graphs {
	prefix := p.GetPrefix()
	items := []mp.Metrics{}

	for _, target := range p.Targets {
		status, ok := p.Statuses[target]
		if !ok {
			continue
		}

		metrics := []mp.Metrics{}
		supports := SupportedMetrics[status.Type]

		for _, support := range supports {
			metrics = append(metrics, mp.Metrics{
				Name:  fmt.Sprintf("%s.%s", target, support.Name),
				Label: support.Name,
			})
		}

		items = append(items, metrics...)
	}

	return map[string]mp.Graphs{
		prefix: {
			Label:   "SwitchBot Metrics",
			Metrics: items,
		},
	}
}

// --------------------
// initialize methods
// --------------------
func main() {
	prefix := flag.String("prefix", "switchbot", "prefix for metrics")
	devices := flag.String("devices", "", "comma separated list of devices to fetch values")
	accessToken := flag.String("token", "", "access token for switchbot api")
	secretToken := flag.String("secret", "", "secret token for switchbot api")
	tempfile := flag.String("tempfile", "", "tempfile")

	flag.Parse()

	c := switchbot.New(*accessToken, *secretToken)

	devicesSlice := strings.Split(*devices, ",")
	sb := SwitchBotPlugin{
		Prefix:          *prefix,
		SwitchBotClient: c,
		Statuses:        map[string]*switchbot.DeviceStatus{},
		Targets:         devicesSlice,
	}

	helper := mp.NewMackerelPlugin(sb)
	helper.Tempfile = *tempfile

	err := sb.FetchStatuses()
	if err != nil {
		log.Fatalln(err)
	}

	helper.Run()
}

// declares

type SwitchBotMetric struct {
	*mp.Metrics
	Unit      string
	ValueFunc func(status *switchbot.DeviceStatus) float64
}

var (
	Battery = &SwitchBotMetric{
		Metrics: &mp.Metrics{
			Name:  "battery",
			Label: "SwitchBot (Battery)",
		},
		Unit: mp.UnitPercentage,
		ValueFunc: func(status *switchbot.DeviceStatus) float64 {
			return float64(status.Battery)
		},
	}

	Temperature = &SwitchBotMetric{
		Metrics: &mp.Metrics{
			Name:  "temperature",
			Label: "SwitchBot (Temperature)"},
		Unit: mp.UnitFloat,
		ValueFunc: func(status *switchbot.DeviceStatus) float64 {
			return status.Temperature
		},
	}

	Humidity = &SwitchBotMetric{
		Metrics: &mp.Metrics{
			Name:  "humidity",
			Label: "SwitchBot (Humidity)",
		},
		Unit: mp.UnitPercentage,
		ValueFunc: func(status *switchbot.DeviceStatus) float64 {
			return float64(status.Humidity)
		},
	}

	CO2 = &SwitchBotMetric{
		Metrics: &mp.Metrics{
			Name:  "co2",
			Label: "SwitchBot (CO2)",
		},
		Unit: mp.UnitInteger,
		ValueFunc: func(status *switchbot.DeviceStatus) float64 {
			return float64(status.CO2)
		},
	}

	ElectricityOfDay = &SwitchBotMetric{
		Metrics: &mp.Metrics{
			Name:  "electricity_of_day",
			Label: "SwitchBot (Electricity of Day)",
		},
		Unit: mp.UnitInteger,
		ValueFunc: func(status *switchbot.DeviceStatus) float64 {
			return float64(status.ElectricityOfDay)
		},
	}

	ElectricCurrent = &SwitchBotMetric{
		Metrics: &mp.Metrics{
			Name:  "electric_current",
			Label: "SwitchBot (Electric Current)",
		},
		Unit: mp.UnitFloat,
		ValueFunc: func(status *switchbot.DeviceStatus) float64 {
			return float64(status.ElectricCurrent)
		},
	}

	Brightness = &SwitchBotMetric{
		Metrics: &mp.Metrics{
			Name:  "brightness",
			Label: "SwitchBot (Brightness)",
		},
		Unit: mp.UnitPercentage,
		ValueFunc: func(status *switchbot.DeviceStatus) float64 {
			value, _ := status.Brightness.Int()
			return float64(value)
		},
	}

	ColorTemperature = &SwitchBotMetric{
		Metrics: &mp.Metrics{
			Name:  "color_temperature",
			Label: "SwitchBot (Color Temperature)",
		},
		Unit: mp.UnitInteger,
		ValueFunc: func(status *switchbot.DeviceStatus) float64 {
			return float64(status.ColorTemperature)
		},
	}

	NebulizationEfficiency = &SwitchBotMetric{
		Metrics: &mp.Metrics{
			Name:  "nebulization_efficiency",
			Label: "SwitchBot (Nebulization Efficiency)",
		},
		Unit: mp.UnitPercentage,
		ValueFunc: func(status *switchbot.DeviceStatus) float64 {
			return float64(status.NebulizationEfficiency)
		},
	}

	FanSpeed = &SwitchBotMetric{
		Metrics: &mp.Metrics{
			Name:  "fan_speed",
			Label: "SwitchBot (Fan Speed)",
		},
		Unit: mp.UnitPercentage,
		ValueFunc: func(status *switchbot.DeviceStatus) float64 {
			return float64(status.FanSpeed)
		},
	}

	SlidePosition = &SwitchBotMetric{
		Metrics: &mp.Metrics{
			Name:  "slide_position",
			Label: "SwitchBot (Slide Position)",
		},
		Unit: mp.UnitPercentage,
		ValueFunc: func(status *switchbot.DeviceStatus) float64 {
			return float64(status.SlidePosition)
		},
	}

	LightLevel = &SwitchBotMetric{
		Metrics: &mp.Metrics{
			Name:  "light_level",
			Label: "SwitchBot (Light Level)",
		},
		Unit: mp.UnitInteger,
		ValueFunc: func(status *switchbot.DeviceStatus) float64 {
			return float64(status.LightLevel)
		},
	}
)

// Unsupported List
// Water Leak Detector
// Mini Robot Vacuum K10+ Pro
// K10+ Pro Combo
// Floor Cleaning Robot S10
// Evaporative Humidifier
// Evaporative Humidifier (Auto-refill)
// Air Purifier VOC
// Air Purifier Table VOC
// Air Purifier PM2.5
// Air Purifier Table PM2.5
// Roller Shade
// Circulator Fan
// Relay Switch 1PM
// Relat Switch 1

var SupportedMetrics = map[switchbot.PhysicalDeviceType][]*SwitchBotMetric{
	switchbot.Bot:                      {Battery},
	switchbot.Curtain:                  {Battery},
	"Curtain3":                         {Battery},
	switchbot.Hub:                      {},
	switchbot.HubPlus:                  {},
	switchbot.HubMini:                  {},
	switchbot.Hub2:                     {Temperature, LightLevel, Humidity},
	switchbot.Meter:                    {Temperature, Battery, Humidity},
	switchbot.MeterPlus:                {Temperature, Battery, Humidity},
	switchbot.MeterPro:                 {Temperature, Battery, Humidity},
	switchbot.MeterProCO2:              {Temperature, Battery, Humidity, CO2},
	switchbot.WoIOSensor:               {Temperature, Battery, Humidity},
	switchbot.Lock:                     {Battery},
	"Smart Lock Pro":                   {Battery},
	switchbot.KeyPad:                   {},
	switchbot.KeyPadTouch:              {},
	switchbot.MotionSensor:             {Battery},
	switchbot.ContactSensor:            {Battery},
	switchbot.CeilingLight:             {Brightness, ColorTemperature},
	switchbot.CeilingLightPro:          {Brightness, ColorTemperature},
	switchbot.PlugMiniUS:               {ElectricityOfDay, ElectricCurrent},
	switchbot.PlugMiniJP:               {ElectricityOfDay, ElectricCurrent},
	switchbot.Plug:                     {},
	switchbot.StripLight:               {Brightness},
	switchbot.ColorBulb:                {Brightness, ColorTemperature},
	switchbot.RobotVacuumCleanerS1:     {Battery},
	switchbot.RobotVacuumCleanerS1Plus: {Battery},
	"K10+":                             {Battery},
	switchbot.Humidifier:               {Humidity, Temperature, NebulizationEfficiency},
	switchbot.BlindTilt:                {SlidePosition},
	"Battery Circulator Fan":           {Battery, FanSpeed},
}
