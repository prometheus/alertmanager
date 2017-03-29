module Alerts.Api exposing (..)

import Alerts.Types exposing (Alert, RouteOpts, Block, AlertGroup)
import Json.Decode as Json exposing (..)
import Utils.Api exposing (baseUrl, iso8601Time)
import Utils.Types exposing (ApiData, Filter)
import Utils.Filter exposing (generateQueryString)


alertGroups : Filter -> Cmd (ApiData (List AlertGroup))
alertGroups filter =
    let
        url =
            String.join "/" [ baseUrl, "alerts", "groups" ++ (generateQueryString filter) ]
    in
        Utils.Api.send (Utils.Api.get url alertGroupsDecoder)



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
        (Json.maybe (field "annotations" (Json.keyValuePairs Json.string)) |> andThen (unwrapWithDefault []))
        (field "labels" (Json.keyValuePairs Json.string))
        (field "inhibited" Json.bool)
        (Json.maybe (field "silenced" Json.string))
        (decodeSilenced)
        (field "startsAt" iso8601Time)
        (field "generatorURL" Json.string)


decodeSilenced : Decoder Bool
decodeSilenced =
    Json.maybe (field "silenced" Json.string)
        |> andThen
            (\val ->
                case val of
                    Just _ ->
                        Json.succeed True

                    Nothing ->
                        Json.succeed False
            )
