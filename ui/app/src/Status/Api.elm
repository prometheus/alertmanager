module Status.Api exposing (getStatus)

import Json.Decode exposing (Decoder, at, bool, field, int, list, map2, maybe, string)
import Status.Types exposing (ClusterPeer, ClusterStatus, StatusResponse, VersionInfo)
import Utils.Api exposing (get, send)
import Utils.Types exposing (ApiData)


getStatus : String -> (ApiData StatusResponse -> msg) -> Cmd msg
getStatus apiUrl msg =
    let
        url =
            String.join "/" [ apiUrl, "status" ]

        request =
            get url decodeStatusResponse
    in
    Cmd.map msg <| send request


decodeStatusResponse : Decoder StatusResponse
decodeStatusResponse =
    field "data" decodeData


decodeData : Decoder StatusResponse
decodeData =
    Json.Decode.map4 StatusResponse
        (field "configYAML" string)
        (field "uptime" string)
        (field "versionInfo" decodeVersionInfo)
        (field "clusterStatus" (maybe decodeClusterStatus))


decodeVersionInfo : Decoder VersionInfo
decodeVersionInfo =
    Json.Decode.map6 VersionInfo
        (field "branch" string)
        (field "buildDate" string)
        (field "buildUser" string)
        (field "goVersion" string)
        (field "revision" string)
        (field "version" string)


decodeClusterStatus : Decoder ClusterStatus
decodeClusterStatus =
    Json.Decode.map3 ClusterStatus
        (field "name" string)
        (field "status" string)
        (field "peers" (list decodeClusterPeer))


decodeClusterPeer : Decoder ClusterPeer
decodeClusterPeer =
    Json.Decode.map2 ClusterPeer
        (field "name" string)
        (field "address" string)
