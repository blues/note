package notelib

import (
	"context"
	"sync"

	"github.com/blues/note-go/notehub"
)

type DiscoverFunc func(edgeUID string, deviceSN string, productUID string, appUID string, needHandlerInfo bool, hostname string, packetHandlerVersion string) (info DiscoverInfo, err error)
type WebRequestFunc func(ctx context.Context, sess *HubSession, deviceUID string, productUID string, alias string, reqtype string, reqcontent string, reqoffset int, reqTotalPayloadLen int, reqRspMaxBytes int, target string, reqLabel string, reqwhen int64, bodyJSON []byte, payload []byte) (rspstatuscode int, rspheader map[string][]string, rspBodyJSON []byte, rspPayloadJSON []byte, err error)
type NoteboxUpdateEnvFunc func(ctx context.Context, ei *NotefileEventInfo, box *Notebox, deviceUID string, appUID string, updateFeeds bool) (err error)
type ReadFileFunc func(ctx context.Context, appUID string, filetype notehub.UploadType, key string, offset int32, length int32, compress bool) (body notehub.UploadMetadata, payload []byte, err error)
type SignalFunc func(deviceUID string, bodyJSON []byte, payload []byte, session *HubSession) (err error)
type GetNotificationFunc func(ctx context.Context, deviceEndpointID string, session *HubSession, eventFn EventFunc) (notifications []string)

// wireLenContextKey is the context key under which the length of a device's web request,
// as a note (see note.Event.WireLen), is passed to the WebRequest handler
type wireLenContextKey struct{}

// ContextWithWireLen returns a context that carries the length of the device's web request,
// as a note, to the WebRequest handler
func ContextWithWireLen(ctx context.Context, wireLen uint32) context.Context {
	return context.WithValue(ctx, wireLenContextKey{}, wireLen)
}

// WireLenFromContext returns the length of the device's web request, as a note, when called
// from within the WebRequest handler, or 0 if unknown
func WireLenFromContext(ctx context.Context) uint32 {
	wireLen, _ := ctx.Value(wireLenContextKey{}).(uint32)
	return wireLen
}

var fnDiscover DiscoverFunc
var fnHubWebRequest WebRequestFunc
var fnNoteboxUpdateEnv NoteboxUpdateEnvFunc
var fnHubReadFile ReadFileFunc
var fnHubSignal SignalFunc
var fnHubNotifications GetNotificationFunc

type Config struct {
	Discover            DiscoverFunc
	WebRequest          WebRequestFunc
	EnvUpdate           NoteboxUpdateEnvFunc
	ReadFile            ReadFileFunc
	Signal              SignalFunc
	DeviceNotifications GetNotificationFunc
	StorageLocation     string
	FileIO              FileIO
}

func Init(config Config) {
	openboxes = sync.Map{}
	fnDiscover = config.Discover
	fnHubWebRequest = config.WebRequest
	fnNoteboxUpdateEnv = config.EnvUpdate
	fnHubReadFile = config.ReadFile
	fnHubSignal = config.Signal
	fnHubNotifications = config.DeviceNotifications
	fsRootStorageLocation = config.StorageLocation
	fio = config.FileIO
	debugEnvInit()
}

func HubSetDiscover(fn DiscoverFunc) {
	fnDiscover = fn
}
