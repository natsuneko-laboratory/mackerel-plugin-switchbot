package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	mp "github.com/mackerelio/go-mackerel-plugin"
	_ "github.com/mattn/go-sqlite3"
	"github.com/nasa9084/go-switchbot/v4"
)

// --------------------
// instance methods
// --------------------

type SwitchBotPlugin struct {
	Prefix          string
	Targets         []string
	SwitchBotClient *switchbot.Client
	CacheDatabase   *sql.DB
}

func (p SwitchBotPlugin) GetDeviceTypeViaDeviceID(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("device id is empty")
	}

	ret, err := p.CacheDatabase.Query("SELECT type FROM sb_device WHERE id = ?", id)
	if err != nil {
		return "", err
	}

	var t string
	for ret.Next() {
		err = ret.Scan(&t)
		if err != nil {
			return "", err
		}

		return t, nil
	}

	return "", nil
}

func (p SwitchBotPlugin) FetchMetrics() (map[string]float64, error) {
	dict := map[string]float64{}

	for _, target := range p.Targets {
		t, err := p.GetDeviceTypeViaDeviceID(target)
		if err != nil {
			continue
		}

		status, err := p.SwitchBotClient.Device().Status(context.Background(), target)
		if err != nil {
			return nil, err
		}

		supports := SupportedMetrics[switchbot.PhysicalDeviceType(t)]

		for _, support := range supports {
			name := fmt.Sprintf("%s.%s", target, support.Name)
			dict[name] = support.ValueFunc(&status)
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
		t, err := p.GetDeviceTypeViaDeviceID(target)
		if err != nil {
			continue
		}

		metrics := []mp.Metrics{}
		supports := SupportedMetrics[switchbot.PhysicalDeviceType(t)]

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

func InitializeDatabase(path string) (*sql.DB, error) {
	if path == "" {
		tmp, err := os.MkdirTemp("", "mackerel-plugin-switchbot")
		if err != nil {
			log.Fatal(err)
			return nil, err
		}

		path = tmp + "/switchbot.db"
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS sb_device (id TEXT PRIMARY KEY, type TEXT, name TEXT, created_at DATETIME, updated_at DATETIME)")
	if err != nil {
		return nil, err
	}

	return db, nil
}

func RefreshDeviceListIfCacheExpired(c *switchbot.Client, db *sql.DB, revalidate uint64) error {
	rows, err := db.Query("SELECT COUNT(id) FROM sb_device")
	if err != nil {
		return err
	}

	defer rows.Close()
	var count uint64
	for rows.Next() {
		err = rows.Scan(&count)
		if err != nil {
			return err
		}
	}

	if revalidate > 0 || count == 0 {
		ret, err := db.Exec(fmt.Sprintf("DELETE FROM sb_device WHERE updated_at < datetime('now', '-%d seconds')", revalidate))
		if err != nil {
			return err
		}

		rowsAffected, err := ret.RowsAffected()
		if err != nil {
			return err
		}

		if rowsAffected > 0 {
			devices, _, _ := c.Device().List(context.Background())

			for _, device := range devices {
				_, err = db.Exec("INSERT OR REPLACE INTO sb_device (id, type, name, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))", device.ID, device.Type, device.Name)
				if err != nil {
					log.Fatalf("%q: %s\n", err, "INSERT OR REPLACE")
					return err
				}
			}
		}
	}

	return nil
}

func GetAllDeviceIds(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SELECT id FROM sb_device")
	if err != nil {
		return nil, err
	}

	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		err = rows.Scan(&id)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, nil
}

func main() {
	prefix := flag.String("prefix", "switchbot", "prefix for metrics")
	path := flag.String("database", "", "cache database for api request")
	devices := flag.String("devices", "", "comma separated list of devices to fetch values")
	revalidate := flag.Uint64("revalidate", 0, "revalidate cache database, 0 is disable")
	accessToken := flag.String("token", "", "access token for switchbot api")
	secretToken := flag.String("secret", "", "secret token for switchbot api")
	tempfile := flag.String("tempfile", "", "tempfile")

	flag.Parse()

	db, err := InitializeDatabase(*path)
	if err != nil {
		log.Fatalf("%q: %s\n", err, "InitializeDatabase")
		return
	}

	defer db.Close()

	c := switchbot.New(*accessToken, *secretToken)
	err = RefreshDeviceListIfCacheExpired(c, db, *revalidate)
	if err != nil {
		log.Fatalf("%q: %s\n", err, "RefreshDeviceListIfCacheExpired")
		return
	}

	devicesSlice := strings.Split(*devices, ",")
	if len(devicesSlice) == 1 && devicesSlice[0] == "" {
		ids, err := GetAllDeviceIds(db)
		if err != nil {
			log.Fatalf("%q: %s\n", err, "GetAllDeviceIds")
		}

		devicesSlice = append(devicesSlice, ids...)
	}

	sb := SwitchBotPlugin{
		Prefix:          *prefix,
		SwitchBotClient: c,
		CacheDatabase:   db,
		Targets:         devicesSlice,
	}

	helper := mp.NewMackerelPlugin(sb)
	helper.Tempfile = *tempfile

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

var SupportedMetrics map[switchbot.PhysicalDeviceType][]*SwitchBotMetric = map[switchbot.PhysicalDeviceType][]*SwitchBotMetric{
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
