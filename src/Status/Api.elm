module Status.Api exposing (getStatus)

import Utils.Api exposing (baseUrl)
import Http
import Status.Types exposing (StatusMsg(NewStatus), StatusResponse)
import Json.Decode exposing (Decoder, map2, string, field, at)
import Types exposing (Msg(MsgForStatus))


getStatus : Cmd Msg
getStatus =
    let
        url =
            String.join "/" [ baseUrl, "status" ]

        request =
            Http.get url decodeStatusResponse
    in
        Http.send (NewStatus >> MsgForStatus) request


decodeStatusResponse : Decoder StatusResponse
decodeStatusResponse =
    map2 StatusResponse
        (field "status" string)
        (at [ "data", "uptime" ] string)
