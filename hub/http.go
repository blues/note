// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// This handler is used to submit JSON requests into the Golang client API for Notecard-like request handling.

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/blues/note-go/note"
	"github.com/blues/note-go/notehub"
	notelib "github.com/blues/note/lib"
)

// Handle inbound HTTP request to the "req" topic
func httpReqHandler(httpRsp http.ResponseWriter, httpReq *http.Request) {
	var err error

	// Allow the default values for these parameters to be set in the URL.
	_, args := httpArgs(httpReq, serverHTTPReqTopic)
	deviceUID := args["device"]
	appUID := args["app"]
	projectUID := args["project"]
	if projectUID != "" {
		appUID = projectUID
	}

	// Get the requestbody
	var reqJSON []byte
	reqJSON, err = ioutil.ReadAll(httpReq.Body)
	if err != nil {
		err = fmt.Errorf("Please supply a JSON request in the HTTP body")
		io.WriteString(httpRsp, string(notelib.ErrorResponse(err)))
		return
	}

	// Debug
	fmt.Printf("http req: %s\n", string(reqJSON))

	// Attempt to extract the appUID and deviceUID from the request, giving these
	// priority over what was in the URL
	req := notehub.HubRequest{}
	err = note.JSONUnmarshal(reqJSON, &req)
	if err == nil {
		if req.DeviceUID != "" {
			deviceUID = req.DeviceUID
		}
		if req.AppUID != "" {
			appUID = req.AppUID
		}
	}

	// Look up the device
	var device DeviceState
	device, err = deviceGet(deviceUID)

	// Get the hub endpoint ID and storage object
	var hubEndpointID, deviceStorageObject string
	if err == nil {
		_, hubEndpointID, _, deviceStorageObject, err = notelib.HubDiscover(deviceUID, "", device.ProductUID)
	}

	// Process the request
	var rspJSON []byte
	if err == nil {
		var box *notelib.Notebox
		box, err = notelib.OpenEndpointNotebox(hubEndpointID, deviceStorageObject, false)
		if err == nil {
			box.SetEventInfo(deviceUID, device.DeviceSN, device.ProductUID, appUID, notehubEvent, nil)
			rspJSON = box.Request(hubEndpointID, reqJSON)
			box.Close()
		}
	}

	// Handle errors
	if err != nil {
		rspJSON = notelib.ErrorResponse(err)
	}

	// Debug
	fmt.Printf("http rsp: %s\n", string(rspJSON))

	// Write the response
	httpRsp.Write(rspJSON)

}

// httpArgs parses the request URI and returns interesting things
func httpArgs(req *http.Request, topic string) (target string, args map[string]string) {
	args = map[string]string{}

	// Trim the request URI
	target = req.RequestURI[len(topic):]

	// If nothing left, there were no args
	if len(target) == 0 {
		return
	}

	// Make sure that the prefix is "/", else the pattern matcher is matching something we don't want
	if strings.HasPrefix(target, "/") {
		target = strings.TrimPrefix(target, "/")
	}

	// See if there is a query, and if so process it
	str := strings.SplitN(target, "?", 2)
	if len(str) == 1 {
		return
	}

	// Now that we know we have args, parse them
	target = str[0]
	values, err := url.ParseQuery(str[1])
	if err != nil {
		return
	}

	// Generate the return arg in the format we expect
	for k, v := range values {
		if len(v) == 1 {
			args[k] = strings.TrimSuffix(strings.TrimPrefix(v[0], "\""), "\"")
		}
	}

	// Done
	return

}
