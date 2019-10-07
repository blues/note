Fork of the golang "json" package because of "bulk template" parsing requirements (see notelib/bulk.go),
with the base being Go 1.12 src/encoding/json
    http://golang.org/dl/go1.12.src.tar.gz

1. Because we need to decompose and reconstruct the JSON template in a linear manner, we need to parse it and reconstruct it with its delimiters in-place.  Unfortunately, the standard JSON Decoder suppresses commas and colons and quotes.  And so jsonxt returns all delimiters, not just []{}

2. Because we are serializing the values without serializing the keys, we need to know in the Decoder when the string it is sending us is a key vs a value.  And so we changed it (in a hacky way) to return keys as "quoted" strings, and values as standard unquoted strings.

```
$ diff stream.go ~/desktop/go1-12/src/encoding/json

1,4d0
< // NOTE that this is a copy of https://golang.org/src/encoding/json/stream.go with the only changes being:
< // 1) Token() always returns the next token, not just []{}.  The XT prefix means "extended token".
< // 2) For string literals that are KEYS, it returns it "quoted" so we can know it's a field, not a value
< //
9c5
< package jsonxt
---
> package json

419,420c415
< 			//			continue
< 			return Delim(':'), nil
---
> 			continue

426,427c421
< 				//				continue
< 				return Delim(','), nil
---
> 				continue

432,433c426
< 				//				continue
< 				return Delim(','), nil
---
> 				continue

448,449c441
< 				//				return x, nil
< 				return "\"" + x + "\"", nil
---
> 				return x, nil

```
