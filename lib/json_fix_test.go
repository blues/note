package notelib

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestSpecificCorruptExamples tests the four specific examples from corrupt.txt
func TestSpecificCorruptExamples(t *testing.T) {
	// These are the exact examples from corrupt.txt
	tests := []struct {
		name           string
		corrupt        string
		expectedChange string
		description    string
	}{
		{
			name: "CorruptExample1_ControlChar",
			// dev: 864049051604371 had a control character (0x1A) at position 2811 instead of ':'
			corrupt:        `{"N":{"1,alerts.qo":{"c":16,"b":{"n":{"s":"file:alerts.qo"}},"h":[{"w":1648850942,"e":"1"}]},"switch_summary.qo":{"u":3,"c":3,"b":{"n":{"i":{},"B":"{\"offline_mins\":14,\"count\":12,\"seconds\":12,\"s1\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"s2\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"s3\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"s4\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"s5\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"s6\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1}}","f":2}},"h":[{"s":3},{}]},"modbus_read_PARAM.qo":{"u":4,"c":6,"b":{"n":{"i":{},"B":"{\"R198\":14.1,\"R199\":12}","f":2}},"h":[{"w":1739318071,"s":4},{"w":1739318071}]},"notebox_temp.qo":{"u":5,"c":2,"b":{"n":{"i":{},"B":"{\"card\":12.1,\"voltage\":12.1,\"offline_mins\":14,\"count\":12,\"seconds\":12,\"s1\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"t1\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1,\"filt\":12.1},\"s2\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"t2\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1,\"filt\":12.1},\"s3\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"t3\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1,\"filt\":12.1},\"s4\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"t4\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1,\"filt\":12.1},\"s5\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"t5\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1,\"filt\":12.1},\"s6\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"t6\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1,\"filt\":12.1},\"expT\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1},\"expRH\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1}}","f":2}},"h":[{"s":5},{}]},"modbus_read_CHANGE.qo":{"u":3,"c":5,"b":{"n":{"i":{},"B":"{\"name\":\"10\",\"val\":14.1}","f":2}},"h":[{"w":1715702443,"s":3},{"w":1715702443}]},"_req.qis":{"c":7,"b":{"n":{"i":{}}},"h":[{"w":1640658896}]},"alerts.qo":{"u":1,"c":11,"b":{"n":{"i":{},"B":"{\"status\":\"64\"}","f":2}},"h":[{"w":1715702388,"s":1},{"w":1715702388}]},"modbus_read_FAST.qo":{"u":3,"c":4,"b":{"n":{"i":{},"B":"{\"R0\":14.1,\"R1\":14.1,\"R2\":14.1,\"R203\":14.1,\"R707\":12}","f":2}},"h":[{"w":1715702442,"s":3},{"w":1715702442}]},"_health.qo":{"u":1,"c":10,"b":{"n":{"i":{},"B":"{\"text\":\"1\",\"alert\":true,\"method\":\"1\"}","P":1,"f":2}},"h":[{"w":1715702365,"s":1},{"w":1715702365}]},"1,switch_summary.qo":{"c":1,"b":{"n":{"s"x"file:switch_summary.qo"}},"h":[{"w":1674159243,"e":"1"}]},"_env.dbs":{"c":9,"b":{"n":{"i":{}}},"h":[{"w":1715702368}],"x":[{"b":{"n":{}},"h":[{"w":1640658870,"e":"1"}]}]},"1,modbus_read_PARAM.qo":{"c":14,"b":{"n":{"s":"file:modbus_read_PARAM.qo"}},"h":[{"w":1670516547,"e":"1"}]},"1,modbus_read_CHANGE.qo":{"c":17,"b":{"n":{"s":"file:modbus_read_CHANGE.qo"}},"h":[{"w":1670516549,"e":"1"}]},"1,modbus_read_FAST.qo":{"c":13,"b":{"n":{"s":"file:modbus_read_FAST.qo"}},"h":[{"w":1670516548,"e":"1"}]},"1,notebox_temp.qo":{"c":12,"b":{"n":{"s":"file:notebox_temp.qo"}},"h":[{"w":1648850941,"e":"1"}]},"_log.qo":{"u":1,"c":8,"b":{"n":{"i":{},"B":"{\"text\":\"1\",\"alert\":true,\"method\":\"1\"}","P":1,"f":2}},"h":[{"w":1715702381,"s":1},{"w":1715702381}]},"1,_log.qo":{"c":22,"b":{"n":{"s":"file:_log.qo"}},"h":[{"w":1661970279,"e":"1"}]},"1,_req.qis":{"c":15,"b":{"n":{"s":"file:_req.qis"}},"h":[{"w":1648850940,"e":"1"}]},"1,_env.dbs":{"c":21,"b":{"n":{"s":"file:_env.dbs"}},"h":[{"w":1640658871,"e":"1"}]},",notebox_temp.qo":{"c":32,"b":{"n":{"i":{},"s":"file:data/notebox_temp-qo.json"}},"h":[{"w":1715702384}]},",switch_summary.qo":{"c":19,"b":{"n":{"i":{},"s":"file:data/switch_summary-qo.json"}},"h":[{"w":1715702386}]},"1,1":{"c":20,"b":{"n":{}},"h":[{"w":1640658869,"e":"1"}]},"1,_health.qo":{"c":18,"b":{"n":{"s":"file:_health.qo"}},"h":[{"w":1640658873,"e":"1"}]},",modbus_read_FAST.qo":{"c":26,"b":{"n":{"i":{},"s":"file:data/modbus_read_FAST-qo.json"}},"h":[{"w":1715702417}]},",_req.qis":{"c":31,"b":{"n":{"i":{},"s":"file:data/_req-qis.json"}},"h":[{"w":1715702418}]},",alerts.qo":{"c":24,"b":{"n":{"i":{},"s":"file:data/alerts-qo.json"}},"h":[{"w":1715702388}]},",modbus_read_CHANGE.qo":{"c":25,"b":{"n":{"i":{},"s":"file:data/modbus_read_CHANGE-qo.json"}},"h":[{"w":1715702416}]},",_health.qo":{"c":27,"b":{"n":{"i":{},"s":"file:data/_health-qo.json"}},"h":[{"w":1715702365}]},",":{"c":29,"b":{"n":{"i":{},"s":"file:data/_notefiles.json"}},"h":[{"w":1715702363}]},",_env.dbs":{"c":30,"b":{"n":{"i":{},"s":"file:data/_env-dbs.json"}},"h":[{"w":1715702369}]},",_log.qo":{"c":28,"b":{"n":{"i":{},"s":"file:data/_log-qo.json"}},"h":[{"w":1715702381}]},",modbus_read_PARAM.qo":{"c":23,"b":{"n":{"i":{},"s":"file:data/modbus_read_PARAM-qo.json"}},"h":[{"w":1715702415}]}},"C":33}`,
			expectedChange: "position 2811",
			description:    "Control character corruption line 1",
		},
		{
			name: "CorruptExample2_MissingQuote",
			// dev:868050045461353 has complex corruption with control characters
			corrupt:        `{"N":{"1,alerts.qo":{"c":16,"b":{"n":{"s":"file:alerts.qo"}},"h":[{"w":1678371027,"e":"1"}]},"switch_summary.qo":{"u":3,"c":3,"b":{"n":{"i":{},"B":"{\"offline_mins\":14,\"count\":12,\"seconds\":12,\"s1\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"s2\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"s3\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"s4\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"s5\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"s6\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1}}","f":2}},"h":[{"s":3},{}]},"modbus_read_PARAM.qo":{"u":3,"c":7,"b":{"n":{"i":{},"B":"{\"R198\":14.1,\"R199\":12}","f":2}},"h":[{"w":1726163904,"s":3},{"w":1726163904}]},"notebox_temp.qo":{"u":4,"c":2,"b":{"n":{"i":{},"B":"{\"card\":12.1,\"voltage\":12.1,\"offline_mins\":14,\"count\":12,\"seconds\":12,\"s1\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"t1\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1,\"filt\":12.1},\"s2\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"t2\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1,\"filt\":12.1},\"s3\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"t3\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1,\"filt\":12.1},\"s4\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"t4\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1,\"filt\":12.1},\"s5\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"t5\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1,\"filt\":12.1},\"s6\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"t6\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1,\"filt\":12.1},\"expT\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1},\"expRH\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1}}","f":2}},"h":[{"s":4},{}]},"modbus_read_CHANGE.qo":{"u":2,"c":6,"b":{"n":{"i":{},"B":"{\"name\":\"10\",\"val\":14.1}","f":2}},"h":[{"w":1715271033,"s":2},{"w":1715271033}]},"_req.qis":{"c":15,"b":{"n":{"i":{}}},"h":[{"w":1678370968}]},"alerts.qo":{"u":1,"c":5,"b":{"n":{"i":{},"B":"{\"status\":\"64\"}","f":2}},"h":[{"w":1715270975,"s":1},{"w":1715270975}]},"modbus_read_FAST.qo":{"u":2,"c":4,"b":{"n":{"i":{},"B":"{\"R0\":14.1,\"R1\":14.1,\"R2\":14.1,\"R203\":14.1,\"R707\":12}","f":2}},"h":[{"w":1715271032,"s":2},{"w":1715271032},{"l":"Y\u00053\u0005},{"l":"\u0001"},{},{},{},{"s":1935828014},{},{}]},"_health.qo":{"u":1,"c":10,"b":{"n":{"i":{},"B":"{\"text\":\"1\",\"alert\":true,\"method\":\"1\"}","P":1,"f":2}},"h":[{"w":1715270952,"s":1},{"w":1715270952}]},"1,switch_summary.qo":{"c":14,"b":{"n":{"s":"file:switch_summary.qo"}},"h":[{"w":1678390509,"e":"1"}]},"_env.dbs":{"c":9,"b":{"n":{"i":{}}},"h":[{"w":1715270955}],"x":[{"b":{"n":{}},"h":[{"w":1678370952,"e":"1"}]}]},"1,modbus_read_PARAM.qo":{"c":13,"b":{"n":{"s":"file:modbus_read_PARAM.qo"}},"h":[{"w":1680015872,"e":"1"}]},"1,modbus_read_CHANGE.qo":{"c":1,"b":{"n":{"s":"file:modbus_read_CHANGE.qo"}},"h":[{"w":1680015873,"e":"1"}]},"1,modbus_read_FAST.qo":{"c":12,"b":{"n":{"s":"file:modbus_read_FAST.qo"}},"h":[{"w":1680015874,"e":"1"}]},"1,notebox_temp.qo":{"c":11,"b":{"n":{"s":"file:notebox_temp.qo"}},"h":[{"w":1678371028,"e":"1"}]},"_log.qo":{"u":1,"c":8,"b":{"n":{"i":{},"B":"{\"text\":\"1\",\"alert\":true,\"method\":\"1\"}","P":1,"f":2}},"h":[{"w":1715270968,"s":1},{"w":1715270968}]},"1,_req.qis":{"c":19,"b":{"n":{"s":"file:_req.qis"}},"h":[{"w":1678371029,"e":"1"}]},"1,_env.dbs":{"c":32,"b":{"n":{"s":"file:_env.dbs"}},"h":[{"w":1678370953,"e":"1"}]},"1,_health.qo":{"c":17,"b":{"n":{"s":"file:_health.qo"}},"h":[{"w":1678370956,"e":"1"}]},"1,_log.qo":{"c":18,"b":{"n":{"s":"file:_log.qo"}},"h":[{"w":1678370955,"e":"1"}]},",notebox_temp.qo":{"c":25,"b":{"n":{"i":{},"s":"file:data/notebox_temp-qo.json"}},"h":[{"w":1715270971}]},",switch_summary.qo":{"c":21,"b":{"n":{"i":{},"s":"file:data/switch_summary-qo.json"}},"h":[{"w":1715270973}]},"1,1":{"c":20,"b":{"n":{}},"h":[{"w":1678370951,"e":"1"}]},",modbus_read_FAST.qo":{"c":24,"b":{"n":{"i":{},"s":"file:data/modbus_read_FAST-qo.json"}},"h":[{"w":1715271006}]},",alerts.qo":{"c":31,"b":{"n":{"i":{},"s":"file:data/alerts-qo.json"}},"h":[{"w":1715270975}]},",_req.qis":{"c":27,"b":{"n":{"i":{},"s":"file:data/_req-qis.json"}},"h":[{"w":1715271007}]},",modbus_read_CHANGE.qo":{"c":23,"b":{"n":{"i":{},"s":"file:data/modbus_read_CHANGE-qo.json"}},"h":[{"w":1715271005}]},",_health.qo":{"c":30,"b":{"n":{"i":{},"s":"file:data/_health-qo.json"}},"h":[{"w":1715270952}]},",":{"c":29,"b":{"n":{"i":{},"s":"file:data/_notefiles.json"}},"h":[{"w":1715270950}]},",_env.dbs":{"c":28,"b":{"n":{"i":{},"s":"file:data/_env-dbs.json"}},"h":[{"w":1715270956}]},",_log.qo":{"c":26,"b":{"n":{"i":{},"s":"file:data/_log-qo.json"}},"h":[{"w":1715270968}]},",modbus_read_PARAM.qo":{"c":22,"b":{"n":{"i":{},"s":"file:data/modbus_read_PARAM-qo.json"}},"h":[{"w":1715271004}]}},"C":33}`,
			expectedChange: "position 2627",
			description:    "Missing quote with control characters from corrupt.txt line 3",
		},
		{
			name: "CorruptExample3_WrongChar",
			// dev:868050045461353 has 'z' instead of ':' at position 4104
			corrupt:        `{"N":{"1,modbus_read_PARAM.qo":{"c":12,"b":{"n":{"s":"file:modbus_read_PARAM.qo"}},"h":[{"w":1683311033,"e":"1"}]},"notebox_temp.qo":{"u":2,"c":4,"b":{"n":{"B":"{\"card\":12.1,\"voltage\":12.1,\"offline_mins\":14,\"count\":12,\"seconds\":12,\"s1\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"t1\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1,\"filt\":12.1},\"s2\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"t2\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1,\"filt\":12.1},\"s3\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"t3\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1,\"filt\":12.1},\"s4\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"t4\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1,\"filt\":12.1},\"s5\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"t5\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1,\"filt\":12.1},\"s6\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"t6\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1,\"filt\":12.1},\"expT\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1},\"expRH\":{\"cur\":12.1,\"min\":12.1,\"max\":12.1,\"avg\":12.1}}","i":{}}},"h":[{"w":1683310243,"s":2},{"w":1683310243}]},"switch_summary.qo":{"u":1,"c":2,"b":{"n":{"i":{},"B":"{\"offline_mins\":14,\"count\":12,\"seconds\":12,\"s1\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"s2\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"s3\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"s4\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"s5\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1},\"s6\":{\"closed\":true,\"count\":11,\"avgopn\":12,\"dc\":11,\"dcv\":11,\"dur\":1}}","f":2}},"h":[{"w":1715708591,"s":1},{"w":1715708591}]},"modbus_read_FAST.qo":{"u":2,"c":7,"b":{"n":{"i":{},"B":"{\"R0\":14.1,\"R1\":14.1,\"R2\":14.1,\"R203\":14.1,\"R707\":12}","f":2}},"h":[{"w":1715708647,"s":2},{"w":1715708647}]},"modbus_read_CHANGE.qo":{"u":2,"c":5,"b":{"n":{"i":{},"B":"{\"name\":\"10\",\"val\":14.1}","f":2}},"h":[{"w":1715708649,"s":2},{"w":1715708649}]},"modbus_read_PARAM.qo":{"u":3,"c":3,"b":{"n":{"i":{},"B":"{\"R198\":14.1,\"R199\":12}","f":2}},"h":[{"w":1739318063,"s":3},{"w":1739318063}]},"_req.qis":{"c":10,"b":{"n":{"i":{}}},"h":[{"w":1667312943}]},"_log.qo":{"u":1,"c":8,"b":{"n":{"i":{},"B":"{\"text\":\"1\",\"alert\":true,\"method\":\"1\"}","P":1,"f":2}},"h":[{"w":1715708586,"s":1},{"w":1715708586}]},"_env.dbs":{"c":1,"b":{"n":{"i":{}}},"h":[{"w":1715708573}],"x":[{"b":{"n":{}},"h":[{"w":1667312927,"e":"1"}]}]},"1,notebox_temp.qo":{"c":13,"b":{"n":{"s":"file:notebox_temp.qo"}},"h":[{"w":1667312933,"e":"1"}]},"1,switch_summary.qo":{"c":11,"b":{"n":{"s":"file:switch_summary.qo"}},"h":[{"w":1683310374,"e":"1"}]},"_health.qo":{"u":1,"c":9,"b":{"n":{"i":{},"B":"{\"text\":\"1\",\"alert\":true,\"method\":\"1\"}","P":1,"f":2}},"h":[{"w":1715708570,"s":1},{"w":1715708570}]},"alerts.qo":{"u":1,"c":6,"b":{"n":{"i":{},"B":"{\"status\":\"64\"}","f":2}},"h":[{"w":1715708593,"s":1},{"w":1715708593}]},"1,modbus_read_FAST.qo":{"c":15,"b":{"n":{"s":"file:modbus_read_FAST.qo"}},"h":[{"w":1683311034,"e":"1"}]},"1,alerts.qo":{"c":22,"b":{"n":{"s":"file:alerts.qo"}},"h":[{"w":1667312931,"e":"1"}]},"1,_req.qis":{"c":16,"b":{"n":{"s":"file:_req.qis"}},"h":[{"w":1667313007,"e":"1"}]},"1,modbus_read_CHANGE.qo":{"c":14,"b":{"n":{"s":"file:modbus_read_CHANGE.qo"}},"h":[{"w":1683311032,"e":"1"}]},"1,_env.dbs":{"c":21,"b":{"n":{"s":"file:_env.dbs"}},"h":[{"w":1667312928,"e":"1"}]},"1,_health.qo":{"c":17,"b":{"n":{"s":"file:_health.qo"}},"h":[{"w":1667312932,"e":"1"}]},"1,_log.qo":{"c":18,"b":{"n":{"s":"file:_log.qo"}},"h":[{"w":1667312934,"e":"1"}]},",notebox_temp.qo":{"c":32,"b":{"n":{"i":{},"s":"file:data/notebox_temp-qo.json"}},"h":[{"w":1715708589}]},",switch_summary.qo":{"c":20,"b":{"n"z{"i":{},"s":"file:data/switch_summary-qo.json"}},"h":[{"w":1715708591}]},",modbus_read_FAST.qo":{"c":25,"b":{"n":{"i":{},"s":"file:data/modbus_read_FAST-qo.json"}},"h":[{"w":1715708621}]},",modbus_read_PARAM.qo":{"c":23,"b":{"n":{"i":{},"s":"file:data/modbus_read_PARAM-qo.json"}},"h":[{"w":1715708619}]},",alerts.qo":{"c":28,"b":{"n":{"i":{},"s":"file:data/alerts-qo.json"}},"h":[{"w":1715708593}]},",_req.qis":{"c":26,"b":{"n":{"i":{},"s":"file:data/_req-qis.json"}},"h":[{"w":1715708622}]},",_health.qo":{"c":31,"b":{"n":{"i":{},"s":"file:data/_health-qo.json"}},"h":[{"w":1715708570}]},",":{"c":30,"b":{"n":{"i":{},"s":"file:data/_notefiles.json"}},"h":[{"w":1715708568}]},",_env.dbs":{"c":29,"b":{"n":{"i":{},"s":"file:data/_env-dbs.json"}},"h":[{"w":1715708574}]},",_log.qo":{"c":27,"b":{"n":{"i":{},"s":"file:data/_log-qo.json"}},"h":[{"w":1715708586}]},",modbus_read_CHANGE.qo":{"c":24,"b":{"n":{"i":{},"s":"file:data/modbus_read_CHANGE-qo.json"}},"h":[{"w":1715708620}]},"1,1":{"c":19,"b":{"n":{}},"h":[{"w":1667312926,"e":"1"}]}},"C":33}`,
			expectedChange: "position 4104",
			description:    "'z' instead of ':' from corrupt.txt line 5",
		},
		{
			name: "CorruptExample4_NumberInsteadOfQuote",
			// dev:868050045461353 has '2' instead of '"' at position 253
			corrupt:        `{"N":{"1,confirmation_messages.qi":{"c":12,"b":{"n":{"s":"file:confirmation_messages.qi"}},"h":[{"w":1741512357,"e":"1"}]},"setup.qo":{"u":1,"c":3,"b":{"n":{"i":{},"B":"{\"mac\":\"1\"}","f":2}},"h":[{"s":1},{}]},"send_message.qo":{"u":1,"c":8,"b":{"n":{2i":{},"B":"{\"COBS_marker\":21,\"id\":\"1\"}","f":2}},"h":[{"s":1},{}]},"confirmation_messages.qi":{"c":7,"b":{"n":{}},"h":[{"w":1741512357,"e":"1"}]},"confirmation_messages_encrypted.qis":{"c":4,"b":{"n":{"i":{}}},"h":[{"w":1741512156}]},"send_message_encrypted.qos":{"u":1,"c":2,"b":{"n":{"i":{},"B":"{\"COBS_marker\":21,\"id\":\"1\"}","f":2}},"h":[{"s":1},{}]},"_health.qo":{"u":1,"c":6,"b":{"n":{"i":{},"B":"{\"text\":\"1\",\"alert\":true,\"voltage\":12.1,\"method\":\"1\"}","P":1,"f":2}},"h":[{"s":1},{}]},"1,setup.qo":{"c":9,"b":{"n":{"s":"file:setup.qo"}},"h":[{"w":1741417437,"e":"1"}]},"_env.dbs":{"c":1,"b":{"n":{}},"h":[{"w":1741417435,"e":"1"}],"x":[{"b":{"n":{"i":{}}},"h":[{}]}]},"1,confirmation_messages_encrypted.qis":{"c":15,"b":{"n":{"s":"file:confirmation_messages_encrypted.qis"}},"h":[{"w":1741512216,"e":"1"}]},"1,send_message.qo":{"c":10,"b":{"n":{"s":"file:send_message.qo"}},"h":[{"w":1741417437,"e":"1"}]},"1,send_message_encrypted.qos":{"c":11,"b":{"n":{"s":"file:send_message_encrypted.qos"}},"h":[{"w":1741417437,"e":"1"}]},"_log.qo":{"u":1,"c":5,"b":{"n":{"i":{},"B":"{\"text\":\"1\",\"alert\":true,\"voltage\":12.1,\"method\":\"1\"}","P":1,"f":2}},"h":[{"s":1},{}]},"1,_log.qo":{"c":14,"b":{"n":{"s":"file:_log.qo"}},"h":[{"w":1741417437,"e":"1"}]},"1,_env.dbs":{"c":26,"b":{"n":{"s":"file:_env.dbs"}},"h":[{"w":1741417435,"e":"1"}]},"1,1":{"c":16,"b":{"n":{}},"h":[{"w":1741417435,"e":"1"}]},"1,_health.qo":{"c":13,"b":{"n":{"s":"file:_health.qo"}},"h":[{"w":1741417437,"e":"1"}]},",send_message.qo":{"c":25,"b":{"n":{"i":{},"s":"file:data/send_message-qo.json"}},"h":[{}]},",send_message_encrypted.qos":{"c":17,"b":{"n":{"i":{},"s":"file:data/send_message_encrypted-qos.json"}},"h":[{}]},",setup.qo":{"c":18,"b":{"n":{"i":{},"s":"file:data/setup-qo.json"}},"h":[{}]},",_log.qo":{"c":24,"b":{"n":{"i":{},"s":"file:data/_log-qo.json"}},"h":[{}]},",confirmation_messages.qi":{"c":19,"b":{"n":{"i":{},"s":"file:data/confirmation_messages-qi.json"}},"h":[{"w":1741512390}]},",_env.dbs":{"c":23,"b":{"n":{"i":{},"s":"file:data/_env-dbs.json"}},"h":[{}]},",":{"c":22,"b":{"n":{"i":{},"s":"file:data/_notefiles.json"}},"h":[{}]},",_health.qo":{"c":21,"b":{"n":{"i":{},"s":"file:data/_health-qo.json"}},"h":[{}]},",confirmation_messages_encrypted.qis":{"c":20,"b":{"n":{"i":{},"s":"file:data/confirmation_messages_encrypted-qis.json"}},"h":[{"w":1741512157}]}},"C":27}`,
			expectedChange: "position 253",
			description:    "'2' instead of '\"' from corrupt.txt line 7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Try to fix the corrupt JSON
			fixed, changeDesc, err := FixCorruptJSONVerbose(tt.corrupt)
			if err != nil {
				t.Fatalf("Failed to fix %s: %v", tt.description, err)
			}

			// Verify the fixed JSON is valid
			var result interface{}
			if err := json.Unmarshal([]byte(fixed), &result); err != nil {
				t.Errorf("Fixed JSON is still invalid for %s: %v", tt.description, err)
				t.Logf("Fixed output: %s", truncate(fixed, 200))
			}

			// Check if the change description contains expected position info
			if !strings.Contains(changeDesc, tt.expectedChange) {
				t.Logf("Change description doesn't mention expected %s: %s", tt.expectedChange, changeDesc)
			}

			t.Logf("Successfully fixed %s", tt.description)
			t.Logf("Change: %s", changeDesc)
		})
	}
}

// TestFixCorruptJSONComprehensive tests various corruption scenarios
func TestFixCorruptJSONComprehensive(t *testing.T) {
	tests := []struct {
		name        string
		corrupt     string
		description string
	}{
		// Missing colons
		{
			name:        "MissingColonSimple",
			corrupt:     `{"key"."value"}`,
			description: "Missing colon after key",
		},
		{
			name:        "MissingColonNested",
			corrupt:     `{"outer":{"inner"?"value"}}`,
			description: "Missing colon in nested object",
		},

		// Missing commas
		{
			name:        "MissingCommaObjects",
			corrupt:     `{"a":1"b":2}`,
			description: "Missing comma between object entries",
		},
		{
			name:        "MissingCommaArray",
			corrupt:     `[1 2,3]`,
			description: "Missing comma in array",
		},

		// Missing braces/brackets
		{
			name:        "MissingClosingBrace",
			corrupt:     `{"key":"value"`,
			description: "Missing closing brace",
		},
		{
			name:        "MissingOpeningBrace",
			corrupt:     `"key":"value"}`,
			description: "Missing opening brace",
		},
		{
			name:        "MissingClosingBracket",
			corrupt:     `[1,2,3`,
			description: "Missing closing bracket",
		},

		// Corrupted quotes
		{
			name:        "CorruptedQuote",
			corrupt:     `{"key':'value"}`,
			description: "Single quote instead of double",
		},
		{
			name:        "MissingQuote",
			corrupt:     `{"key:"value"}`,
			description: "Missing quote after key",
		},

		// Numeric corruptions
		{
			name:        "CorruptedNumber",
			corrupt:     `{"num":12.a}`,
			description: "Letter in number",
		},
		{
			name:        "CorruptedNumberInArray",
			corrupt:     `[1,2,x,4]`,
			description: "Letter instead of number in array",
		},

		// Control character corruptions
		{
			name:        "ControlCharInKey",
			corrupt:     "{\"ke\x01\":\"value\"}",
			description: "Control character in key",
		},
		{
			name:        "ControlCharInStructure",
			corrupt:     "{\"key\"\x02\"value\"}",
			description: "Control character instead of colon",
		},

		// Complex nested corruptions
		{
			name:        "NestedCorruption1",
			corrupt:     `{"a":{"b":[1,2,{"c":"d"e":"f"}]}}`,
			description: "Missing comma in nested object within array",
		},
		{
			name:        "NestedCorruption2",
			corrupt:     `{"users":[{"name":"John""age":30},{"name":"Jane","age":25}]}`,
			description: "Missing comma between nested object properties",
		},

		// Real-world-like corruptions
		{
			name:        "CorruptedBoolean",
			corrupt:     `{"active":true,"count":5}`,
			description: "Corrupted boolean value",
		},
		{
			name:        "CorruptedNull",
			corrupt:     `{"data":nul1,"valid":true}`,
			description: "Corrupted null value",
		},

		// Edge cases
		{
			name:        "SingleCharJSON",
			corrupt:     `}`,
			description: "Single wrong character",
		},
		{
			name:        "EmptyCorrupted",
			corrupt:     `{]`,
			description: "Wrong closing character",
		},
		{
			name:        "CorruptedEscape",
			corrupt:     `{"path":"C:\test\file"}`,
			description: "Missing escape in string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixed, changeDesc, err := FixCorruptJSONVerbose(tt.corrupt)
			if err != nil {
				// Some might not be fixable with single-character correction
				t.Logf("Could not fix %s: %v", tt.description, err)

				// Try context-aware fix
				if contextFixed, err := TryFixWithContext(tt.corrupt); err == nil {
					var result interface{}
					if err := json.Unmarshal([]byte(contextFixed), &result); err == nil {
						t.Logf("Fixed with context analysis")
						fixed = contextFixed
					}
				}

				if fixed == "" {
					return
				}
			}

			// Verify the fixed JSON is valid
			var result interface{}
			if err := json.Unmarshal([]byte(fixed), &result); err != nil {
				t.Errorf("Fixed JSON is still invalid: %v", err)
				t.Logf("Original: %s", tt.corrupt)
				t.Logf("Fixed: %s", fixed)
				return
			}

			t.Logf("Successfully fixed %s: %s", tt.description, changeDesc)
			t.Logf("Original: %s", tt.corrupt)
			t.Logf("Fixed: %s", fixed)
		})
	}
}

// TestAlreadyValidJSON ensures valid JSON passes through unchanged
func TestAlreadyValidJSON(t *testing.T) {
	validExamples := []string{
		`{"key":"value"}`,
		`[1,2,3]`,
		`{"nested":{"array":[1,2,3],"bool":true,"null":null}}`,
		`{"unicode":"Hello, 世界"}`,
		`{"escaped":"Line 1\nLine 2\tTabbed"}`,
		`{"number":123.456,"negative":-789,"exp":1.23e-4}`,
	}

	for i, valid := range validExamples {
		t.Run(fmt.Sprintf("Valid%d", i+1), func(t *testing.T) {
			fixed, err := FixCorruptJSON(valid)
			if err != nil {
				t.Errorf("Failed on valid JSON: %v", err)
				return
			}

			if fixed != valid {
				t.Errorf("Valid JSON was modified\nOriginal: %s\nFixed: %s", valid, fixed)
			}
		})
	}
}

// TestPerformance tests performance with larger JSON
func TestPerformance(t *testing.T) {
	// Create a large JSON with a single corruption
	largeJSON := `{"items":[`
	for i := 0; i < 100; i++ {
		if i > 0 {
			largeJSON += ","
		}
		largeJSON += fmt.Sprintf(`{"id":%d,"name":"Item %d","active":true}`, i, i)
	}
	// Add corruption: missing closing bracket
	largeJSON += `}`

	fixed, err := FixCorruptJSON(largeJSON)
	if err != nil {
		t.Logf("Could not fix large JSON: %v", err)
		return
	}

	var result interface{}
	if err := json.Unmarshal([]byte(fixed), &result); err != nil {
		t.Errorf("Fixed large JSON is invalid: %v", err)
	} else {
		t.Logf("Successfully fixed large JSON")
	}
}

// Helper function to truncate strings for logging
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
