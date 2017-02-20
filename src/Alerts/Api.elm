module Alerts.Api exposing (..)

import Json.Decode as Json exposing (..)
import Utils.Api exposing (baseUrl, stringtoISO8601)
import Alerts.Types exposing (..)


getAlertGroups : Cmd Msg
getAlertGroups =
    let
        url =
            String.join "/" [ baseUrl, "alerts", "groups" ]
    in
        Utils.Api.send (Utils.Api.get url alertGroupsDecoder)
            |> Cmd.map (AlertGroupsFetch >> ForSelf)



-- Decoders
-- Once the API returns the newly created silence, this can go away and we
-- re-use the silence decoder.


alertGroupsDecoder : Json.Decoder (List AlertGroup)
alertGroupsDecoder =
    Json.at [ "data" ] (Json.list alertGroupDecoder)


alertGroupDecoder : Json.Decoder AlertGroup
alertGroupDecoder =
    Json.map2 AlertGroup
        (decodeBlocks)
        (Json.at [ "labels" ] (Json.keyValuePairs Json.string))


decodeBlocks : Json.Decoder (List Block)
decodeBlocks =
    Json.maybe (field "blocks" (Json.list blockDecoder))
        |> andThen (unwrapWithDefault [])


unwrapWithDefault : a -> Maybe a -> Json.Decoder a
unwrapWithDefault default val =
    case val of
        Just a ->
            Json.succeed a

        Nothing ->
            Json.succeed default


blockDecoder : Json.Decoder Block
blockDecoder =
    Json.map2 Block
        (field "alerts" <| Json.list alertDecoder)
        (field "routeOpts" routeOptsDecoder)


routeOptsDecoder : Json.Decoder RouteOpts
routeOptsDecoder =
    Json.map RouteOpts
        (field "receiver" Json.string)


alertDecoder : Json.Decoder Alert
alertDecoder =
    Json.map7 Alert
        (field "annotations" (Json.keyValuePairs Json.string))
        (field "labels" (Json.keyValuePairs Json.string))
        (field "inhibited" Json.bool)
        (Json.maybe (field "silenced" Json.int))
        (decodeSilenced)
        (field "startsAt" stringtoISO8601)
        (field "generatorURL" Json.string)


decodeSilenced : Decoder Bool
decodeSilenced =
    Json.maybe (field "silenced" Json.int)
        |> andThen
            (\val ->
                case val of
                    Just _ ->
                        Json.succeed True

                    Nothing ->
                        Json.succeed False
            )
