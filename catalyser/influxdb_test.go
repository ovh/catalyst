package catalyser

import (
	"fmt"
	"testing"
)

func TestParseInflux(t *testing.T) {
	tests := []struct {
		Got             string
		ExpectClassname string
		ExpectLabels    map[string]string
	}{
		{
			"string,hostname=localhost a=\" b \"",
			"string.a",
			map[string]string{
				"hostname": "localhost",
			},
		},
		{
			"string,hostname=localhost a=\" b \",c=\"d\" 1434055562000000000",
			"string.a",
			map[string]string{
				"hostname": "localhost",
			},
		},
		//
		// cpu,hostname=localhost total=100,used=10,free=90
		{
			"cpu_load_short,host=server01,region=us-west value=0.64 1434055562000000000",
			"cpu_load_short.value",
			map[string]string{
				"host":            "server01",
				"region":          "us-west",
				".influxdb_field": "value",
			},
		},
		{
			"system,client=curanobis,host=staging.curanobis.com uptime_format=\"59 days, 18:23\" 1515597480000000000",
			"system.uptime_format",
			map[string]string{
				"client": "curanobis",
				"host":   "staging.curanobis.com",
			},
		},

		{
			"weather,location=us-midwest temperature=82,bug_concentration=98 1465839830100400200",
			"weather.temperature",
			map[string]string{
				"location": "us-midwest",
				"host":     "staging.curanobis.com",
			},
		},
		{
			"weather,location=us-midwest temperature=82,bug_concentration=98,test=\"the answer is equal to 42 with a ,\" 1465839830100400200",
			"weather.temperature",
			map[string]string{
				"location": "us-midwest",
				"host":     "staging.curanobis.com",
			},
		},
		{
			"bridges,type=suspension visitors=234 1478133071000000000",
			"bridges.visitors",
			map[string]string{
				"type": "suspension",
			},
		},
	}

	for _, test := range tests {
		fmt.Println("testing ", test.Got)
		gts, err := parseInflux([]byte(test.Got), "n")
		if err != nil {
			t.Error(err)
		}
		if gts[0].Name != test.ExpectClassname {
			t.Error("Wrong classname for ", gts[0].Name)
		}

		for _, singleGTS := range gts {
			fmt.Printf("name: '%v', labels: '%+v', value: '%v'\n", singleGTS.Name, singleGTS.Labels, singleGTS.Value)
		}

		fmt.Println("=======")

		labels := gts[0].Labels
		for labelKey, labelValue := range labels {
			if test.ExpectLabels[labelKey] != labelValue {
				t.Errorf("label %v is wrong, expected %v, got %v", labelKey, labelValue, test.ExpectLabels[labelKey])
			}
		}

	}

}
