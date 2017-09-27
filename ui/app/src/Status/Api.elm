module Status.Api exposing (getStatus)

import Utils.Api exposing (send, get, (|:))
import Utils.Types exposing (ApiData, Matcher)
import Status.Types exposing (StatusResponse, VersionInfo, MeshStatus, MeshPeer, Route)
import Json.Decode
    exposing
        ( Decoder
        , map
        , map2
        , map3
        , map5
        , map6
        , string
        , succeed
        , field
        , at
        , list
        , int
        , maybe
        , bool
        , keyValuePairs
        , lazy
        )


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
    map5 StatusResponse
        (field "configYAML" string)
        (field "uptime" string)
        (field "versionInfo" decodeVersionInfo)
        (field "meshStatus" (maybe decodeMeshStatus))
        (at [ "configJSON", "route" ] decodeRoute)


decodeVersionInfo : Decoder VersionInfo
decodeVersionInfo =
    map6 VersionInfo
        (field "branch" string)
        (field "buildDate" string)
        (field "buildUser" string)
        (field "goVersion" string)
        (field "revision" string)
        (field "version" string)


decodeMeshStatus : Decoder MeshStatus
decodeMeshStatus =
    map3 MeshStatus
        (field "name" string)
        (field "nickName" string)
        (field "peers" (list decodeMeshPeer))


decodeMeshPeer : Decoder MeshPeer
decodeMeshPeer =
    map3 MeshPeer
        (field "name" string)
        (field "nickName" string)
        (field "uid" int)


decodeRoute : Decoder Route
decodeRoute =
    succeed Route
        |: (maybe (field "receiver" string))
        |: (maybe (field "group_by" (list string)))
        |: (maybe (field "continue" bool))
        |: matchers
        |: (maybe (field "group_wait" int))
        |: (maybe (field "group_interval" int))
        |: (maybe (field "repeat_interval" int))
        |: (maybe (field "routes" (map Status.Types.Routes (list (lazy (\_ -> decodeRoute))))))
        |: (succeed Nothing)
        |: (succeed 0)
        |: (succeed 0)
        |: (succeed 0)


matchers : Decoder (List Matcher)
matchers =
    map2
        (\matchers matcher_res ->
            let
                m =
                    matchers
                        |> Maybe.withDefault []
                        |> List.map (\( name, value ) -> { isRegex = False, name = name, value = value })

                mRe =
                    matcher_res
                        |> Maybe.withDefault []
                        |> List.map (\( name, value ) -> { isRegex = True, name = name, value = value })
            in
                m ++ mRe
        )
        (maybe (field "match" (keyValuePairs string)))
        (maybe (field "match_re" (keyValuePairs string)))
