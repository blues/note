// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package notelib

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

const verboseSmazzTest = false

type smazzTest struct {
	name string
	test []byte
	len  int
}

var smazzTests = []smazzTest{

	{
		name: "test-null",
		test: []byte{},
		len:  0,
	},

	{
		name: "test-grow",
		test: []byte("AAAAAaBBBBBbCCCCCcDDDDDdEEEEEeFFFFFf"),
		len:  36,
	},

	{
		name: "test1",
		test: []byte(`{"p":"net.ozzie.ray:test","s":"my device","k":"NOTE-LORA,"v":"1.2.3","o":""}`),
		len:  48,
	},

	{
		name: "test2",
		test: []byte(`{"minutes":12,"v_min":12.1,"v_max":12.1,"v_chg":12.1,"i_min":12.1,"i_max":12.1,"i_chg":12.1,"p_min":12.1,"p_max":12.1,"p_chg":12.1}`),
		len:  38,
	},

	{
		name: "test3",
		test: []byte(`{"status":"0","motion":12,"seconds":14,"time":14,"dop":12.1,"voltage":12.1,"daily_charging_mins":14,"distance":14.1,"bearing":14.1,"velocity":14.1,"temperature":12.1,"humidity":12.1,"pressure":14.1,"total":12,"count":12,"sensor":"12","usv":14.1,"cpm":12.1,"cpm_secs":12,"cpm_count":14,"journey":14,"jcount":12,"button":true,"inside_fence":true,"usb":true,"charging":true}`),
		len:  145,
	},

	{
		name: "test4",
		test: []byte(`{"voltage":12.1,"temperature":14.1,"humidity":14.1,"pressure":14.1,"motion":12,"count":14}`),
		len:  35,
	},

	{
		name: "test5",
		test: []byte(`{"sensor":"12","pm01_0":14.1,"pm02_5":14.1,"pm10_0":14.1,"pm01_0_rstd":12.1,"pm02_5_rstd":12.1,"pm10_0_rstd":12.1,"c00_30":12,"c00_50":12,"c01_00":12,"c02_50":12,"c05_00":12,"c10_00":12,"pm01_0cf1":14.1,"pm02_5cf1":14.1,"pm10_0cf1":14.1,"csamples":12,"csecs":12,"voltage":12.1,"temperature":14.1,"humidity":14.1,"pressure":14.1,"charging":true,"usb":true,"indoors":true,"motion":12,"cpm":12.1,"cpm_count":14,"usv":12.1}`),
		len:  203,
	},
}

func TestSmazz(t *testing.T) {

	ctx := Smazz(SmazzCodeTemplate)
	for _, test := range smazzTests {
		compressed, err := ctx.Encode(nil, test.test)
		require.NoError(t, err)
		isUncompressed := ""
		if len(test.test) == 0 || compressed[0] != 0 {
			isUncompressed = "(would have grown)"
		}
		decompressed, err := ctx.Decode(nil, compressed)
		require.NoError(t, err)
		pct := float64(0)
		if len(test.test) > 0 {
			pct = float64(len(test.test)-len(compressed)) * 100.0 / float64(len(test.test))
		}
		if verboseSmazzTest {
			fmt.Printf("smazz_test name:%s inp:%d enc:%d dec:%d pct:%f %s\n", test.name, len(test.test), len(compressed), len(decompressed), pct, isUncompressed)
		}
		require.Equal(t, decompressed, test.test)
		require.Equal(t, len(compressed), test.len)
	}

}
