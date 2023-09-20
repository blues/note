/* Automatically generated nanopb header */
/* Generated by nanopb-0.3.5 at Wed Apr 26 09:06:14 2023. */

#ifndef PB_NOTEHUB_PB_H_INCLUDED
#define PB_NOTEHUB_PB_H_INCLUDED
#include <pb.h>

/* @@protoc_insertion_point(includes) */
#if PB_PROTO_HEADER_VERSION != 30
#error Regenerate this file with the current version of nanopb generator.
#endif

#ifdef __cplusplus
extern "C" {
#endif

/* Struct definitions */
typedef struct _notelib_NotehubPB {
    bool has_Version;
    int64_t Version;
    bool has_MessageType;
    char MessageType[25];
    bool has_Error;
    char Error[254];
    bool has_DeviceUID;
    char DeviceUID[100];
    bool has_DeviceEndpointID;
    char DeviceEndpointID[100];
    bool has_HubTimeNs;
    int64_t HubTimeNs;
    bool has_HubEndpointID;
    char HubEndpointID[100];
    bool has_HubSessionTicket;
    char HubSessionTicket[128];
    bool has_HubSessionHandler;
    char HubSessionHandler[254];
    bool has_HubSessionTicketExpiresTimeSec;
    int64_t HubSessionTicketExpiresTimeSec;
    bool has_NotefileID;
    char NotefileID[254];
    bool has_NotefileIDs;
    char NotefileIDs[254];
    bool has_Since;
    int64_t Since;
    bool has_Until;
    int64_t Until;
    bool has_MaxChanges;
    int64_t MaxChanges;
    bool has_DeviceSN;
    char DeviceSN[254];
    bool has_NoteID;
    char NoteID[254];
    bool has_SessionIDPrev;
    int64_t SessionIDPrev;
    bool has_SessionIDNext;
    int64_t SessionIDNext;
    bool has_SessionIDMismatch;
    bool SessionIDMismatch;
    bool has_Bytes1;
    int64_t Bytes1;
    bool has_Bytes2;
    int64_t Bytes2;
    bool has_Bytes3;
    int64_t Bytes3;
    bool has_Bytes4;
    int64_t Bytes4;
    bool has_ProductUID;
    char ProductUID[254];
    bool has_UsageProvisioned;
    int64_t UsageProvisioned;
    bool has_UsageRcvdBytes;
    uint32_t UsageRcvdBytes;
    bool has_UsageSentBytes;
    uint32_t UsageSentBytes;
    bool has_UsageTCPSessions;
    uint32_t UsageTCPSessions;
    bool has_UsageTLSSessions;
    uint32_t UsageTLSSessions;
    bool has_UsageRcvdNotes;
    uint32_t UsageRcvdNotes;
    bool has_UsageSentNotes;
    uint32_t UsageSentNotes;
    bool has_CellID;
    char CellID[254];
    bool has_NotificationSession;
    bool NotificationSession;
    bool has_Voltage100;
    int32_t Voltage100;
    bool has_Temp100;
    int32_t Temp100;
    bool has_ContinuousSession;
    bool ContinuousSession;
    bool has_MotionSecs;
    int64_t MotionSecs;
    bool has_MotionOrientation;
    char MotionOrientation[100];
    bool has_SessionTrigger;
    char SessionTrigger[254];
    bool has_Voltage1000;
    int32_t Voltage1000;
    bool has_Temp1000;
    int32_t Temp1000;
    bool has_HubSessionFactoryResetID;
    char HubSessionFactoryResetID[100];
    bool has_HighPowerSecsTotal;
    uint32_t HighPowerSecsTotal;
    bool has_HighPowerSecsData;
    uint32_t HighPowerSecsData;
    bool has_HighPowerSecsGPS;
    uint32_t HighPowerSecsGPS;
    bool has_HighPowerCyclesTotal;
    uint32_t HighPowerCyclesTotal;
    bool has_HighPowerCyclesData;
    uint32_t HighPowerCyclesData;
    bool has_HighPowerCyclesGPS;
    uint32_t HighPowerCyclesGPS;
    bool has_DeviceSKU;
    char DeviceSKU[100];
    bool has_DeviceFirmware;
    int64_t DeviceFirmware;
    bool has_DevicePIN;
    char DevicePIN[254];
    bool has_DeviceOrderingCode;
    char DeviceOrderingCode[254];
    bool has_UsageRcvdBytesSecondary;
    uint32_t UsageRcvdBytesSecondary;
    bool has_UsageSentBytesSecondary;
    uint32_t UsageSentBytesSecondary;
    bool has_SuppressResponse;
    bool SuppressResponse;
    bool has_Where;
    char Where[25];
    bool has_WhereWhen;
    int64_t WhereWhen;
/* @@protoc_insertion_point(struct:notelib_NotehubPB) */
} notelib_NotehubPB;

/* Default values for struct fields */

/* Initializer values for message structs */
#define notelib_NotehubPB_init_default           {false, 0, false, "", false, "", false, "", false, "", false, 0, false, "", false, "", false, "", false, 0, false, "", false, "", false, 0, false, 0, false, 0, false, "", false, "", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", false, 0, false, 0, false, 0, false, 0, false, 0, false, "", false, "", false, 0, false, 0, false, "", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", false, 0, false, "", false, "", false, 0, false, 0, false, 0, false, "", false, 0}
#define notelib_NotehubPB_init_zero              {false, 0, false, "", false, "", false, "", false, "", false, 0, false, "", false, "", false, "", false, 0, false, "", false, "", false, 0, false, 0, false, 0, false, "", false, "", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", false, 0, false, 0, false, 0, false, 0, false, 0, false, "", false, "", false, 0, false, 0, false, "", false, 0, false, 0, false, 0, false, 0, false, 0, false, 0, false, "", false, 0, false, "", false, "", false, 0, false, 0, false, 0, false, "", false, 0}

/* Field tags (for use in manual encoding/decoding) */
#define notelib_NotehubPB_Version_tag            1
#define notelib_NotehubPB_MessageType_tag        2
#define notelib_NotehubPB_Error_tag              3
#define notelib_NotehubPB_DeviceUID_tag          4
#define notelib_NotehubPB_DeviceEndpointID_tag   5
#define notelib_NotehubPB_HubTimeNs_tag          6
#define notelib_NotehubPB_HubEndpointID_tag      7
#define notelib_NotehubPB_HubSessionTicket_tag   8
#define notelib_NotehubPB_HubSessionHandler_tag  9
#define notelib_NotehubPB_HubSessionTicketExpiresTimeSec_tag 10
#define notelib_NotehubPB_NotefileID_tag         11
#define notelib_NotehubPB_NotefileIDs_tag        12
#define notelib_NotehubPB_Since_tag              13
#define notelib_NotehubPB_Until_tag              14
#define notelib_NotehubPB_MaxChanges_tag         15
#define notelib_NotehubPB_DeviceSN_tag           16
#define notelib_NotehubPB_NoteID_tag             17
#define notelib_NotehubPB_SessionIDPrev_tag      18
#define notelib_NotehubPB_SessionIDNext_tag      19
#define notelib_NotehubPB_SessionIDMismatch_tag  20
#define notelib_NotehubPB_Bytes1_tag             21
#define notelib_NotehubPB_Bytes2_tag             22
#define notelib_NotehubPB_Bytes3_tag             23
#define notelib_NotehubPB_Bytes4_tag             24
#define notelib_NotehubPB_ProductUID_tag         25
#define notelib_NotehubPB_UsageProvisioned_tag   26
#define notelib_NotehubPB_UsageRcvdBytes_tag     27
#define notelib_NotehubPB_UsageSentBytes_tag     28
#define notelib_NotehubPB_UsageTCPSessions_tag   29
#define notelib_NotehubPB_UsageTLSSessions_tag   30
#define notelib_NotehubPB_UsageRcvdNotes_tag     31
#define notelib_NotehubPB_UsageSentNotes_tag     32
#define notelib_NotehubPB_CellID_tag             33
#define notelib_NotehubPB_NotificationSession_tag 34
#define notelib_NotehubPB_Voltage100_tag         35
#define notelib_NotehubPB_Temp100_tag            36
#define notelib_NotehubPB_ContinuousSession_tag  37
#define notelib_NotehubPB_MotionSecs_tag         38
#define notelib_NotehubPB_MotionOrientation_tag  39
#define notelib_NotehubPB_SessionTrigger_tag     40
#define notelib_NotehubPB_Voltage1000_tag        41
#define notelib_NotehubPB_Temp1000_tag           42
#define notelib_NotehubPB_HubSessionFactoryResetID_tag 43
#define notelib_NotehubPB_HighPowerSecsTotal_tag 44
#define notelib_NotehubPB_HighPowerSecsData_tag  45
#define notelib_NotehubPB_HighPowerSecsGPS_tag   46
#define notelib_NotehubPB_HighPowerCyclesTotal_tag 47
#define notelib_NotehubPB_HighPowerCyclesData_tag 48
#define notelib_NotehubPB_HighPowerCyclesGPS_tag 49
#define notelib_NotehubPB_DeviceSKU_tag          50
#define notelib_NotehubPB_DeviceFirmware_tag     51
#define notelib_NotehubPB_DevicePIN_tag          52
#define notelib_NotehubPB_DeviceOrderingCode_tag 53
#define notelib_NotehubPB_UsageRcvdBytesSecondary_tag 54
#define notelib_NotehubPB_UsageSentBytesSecondary_tag 55
#define notelib_NotehubPB_SuppressResponse_tag   56
#define notelib_NotehubPB_Where_tag              57
#define notelib_NotehubPB_WhereWhen_tag          58

/* Struct field encoding specification for nanopb */
extern const pb_field_t notelib_NotehubPB_fields[59];

/* Maximum encoded size of messages (where known) */
#define notelib_NotehubPB_size                   3979

/* Message IDs (where set with "msgid" option) */
#ifdef PB_MSGID

#define NOTEHUB_MESSAGES \


#endif

#ifdef __cplusplus
} /* extern "C" */
#endif
/* @@protoc_insertion_point(eof) */

#endif
