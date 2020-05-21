/* Automatically generated nanopb constant definitions */
/* Generated by nanopb-0.3.5 at Thu May 14 07:09:02 2020. */

#include "notehub.pb.h"

/* @@protoc_insertion_point(includes) */
#if PB_PROTO_HEADER_VERSION != 30
#error Regenerate this file with the current version of nanopb generator.
#endif



const pb_field_t notelib_NotehubPB_fields[38] = {
    PB_FIELD(  1, INT64   , OPTIONAL, STATIC  , FIRST, notelib_NotehubPB, Version, Version, 0),
    PB_FIELD(  2, STRING  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, MessageType, Version, 0),
    PB_FIELD(  3, STRING  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, Error, MessageType, 0),
    PB_FIELD(  4, STRING  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, DeviceUID, Error, 0),
    PB_FIELD(  5, STRING  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, DeviceEndpointID, DeviceUID, 0),
    PB_FIELD(  6, INT64   , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, HubTimeNs, DeviceEndpointID, 0),
    PB_FIELD(  7, STRING  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, HubEndpointID, HubTimeNs, 0),
    PB_FIELD(  8, STRING  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, HubSessionTicket, HubEndpointID, 0),
    PB_FIELD(  9, STRING  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, HubSessionHandler, HubSessionTicket, 0),
    PB_FIELD( 10, INT64   , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, HubSessionTicketExpiresTimeSec, HubSessionHandler, 0),
    PB_FIELD( 11, STRING  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, NotefileID, HubSessionTicketExpiresTimeSec, 0),
    PB_FIELD( 12, STRING  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, NotefileIDs, NotefileID, 0),
    PB_FIELD( 13, INT64   , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, Since, NotefileIDs, 0),
    PB_FIELD( 14, INT64   , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, Until, Since, 0),
    PB_FIELD( 15, INT64   , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, MaxChanges, Until, 0),
    PB_FIELD( 16, STRING  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, DeviceSN, MaxChanges, 0),
    PB_FIELD( 17, STRING  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, NoteID, DeviceSN, 0),
    PB_FIELD( 18, INT64   , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, SessionIDPrev, NoteID, 0),
    PB_FIELD( 19, INT64   , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, SessionIDNext, SessionIDPrev, 0),
    PB_FIELD( 20, BOOL    , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, SessionIDMismatch, SessionIDNext, 0),
    PB_FIELD( 21, INT64   , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, Bytes1, SessionIDMismatch, 0),
    PB_FIELD( 22, INT64   , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, Bytes2, Bytes1, 0),
    PB_FIELD( 23, INT64   , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, Bytes3, Bytes2, 0),
    PB_FIELD( 24, INT64   , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, Bytes4, Bytes3, 0),
    PB_FIELD( 25, STRING  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, ProductUID, Bytes4, 0),
    PB_FIELD( 26, INT64   , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, UsageProvisioned, ProductUID, 0),
    PB_FIELD( 27, UINT32  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, UsageRcvdBytes, UsageProvisioned, 0),
    PB_FIELD( 28, UINT32  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, UsageSentBytes, UsageRcvdBytes, 0),
    PB_FIELD( 29, UINT32  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, UsageTCPSessions, UsageSentBytes, 0),
    PB_FIELD( 30, UINT32  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, UsageTLSSessions, UsageTCPSessions, 0),
    PB_FIELD( 31, UINT32  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, UsageRcvdNotes, UsageTLSSessions, 0),
    PB_FIELD( 32, UINT32  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, UsageSentNotes, UsageRcvdNotes, 0),
    PB_FIELD( 33, STRING  , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, CellID, UsageSentNotes, 0),
    PB_FIELD( 34, BOOL    , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, NotificationSession, CellID, 0),
    PB_FIELD( 35, INT32   , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, Voltage100, NotificationSession, 0),
    PB_FIELD( 36, INT32   , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, Temp100, Voltage100, 0),
    PB_FIELD( 37, BOOL    , OPTIONAL, STATIC  , OTHER, notelib_NotehubPB, ContinuousSession, Temp100, 0),
    PB_LAST_FIELD
};


/* @@protoc_insertion_point(eof) */
