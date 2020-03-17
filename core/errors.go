package core

// InfluxDBParseError influxDB parsing error
type InfluxDBParseError struct {
	Err string `json:"error"`
}

func (e InfluxDBParseError) Error() string {
	return e.Err
}
