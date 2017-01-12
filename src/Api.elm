module Api exposing (..)

-- External Imports

import Http
import Json.Decode as Json exposing (..)
import Task
import String
import Date exposing (..)
import ISO8601


-- Internal Imports

import Types exposing (..)


-- Api


baseUrl : String
baseUrl =
    "http://localhost:9093/api/v1"


getSilences : Cmd Msg
getSilences =
    let
        url =
            String.join "/" [ baseUrl, "silences?limit=1000" ]
    in
        Http.send SilencesFetch (Http.get url listResponseDecoder)


getSilence : Int -> Cmd Msg
getSilence id =
    let
        url =
            String.join "/" [ baseUrl, "silence", toString id ]
    in
        Http.send SilenceFetch (Http.get url showResponseDecoder)


getAlertGroups : Cmd Msg
getAlertGroups =
    let
        url =
            String.join "/" [ baseUrl, "alerts", "groups" ]
    in
        Http.send AlertGroupsFetch (Http.get url alertGroupsDecoder)



-- Make these generic when I've gotten to Alerts


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


stringtoISO8601 : Decoder ISO8601.Time
stringtoISO8601 =
    Json.string
        |> andThen
            (\val ->
                case ISO8601.fromString val of
                    Err err ->
                        Json.fail err

                    Ok date ->
                        Json.succeed <| date
            )


showResponseDecoder : Json.Decoder Silence
showResponseDecoder =
    (Json.at [ "data" ] silenceDecoder)


listResponseDecoder : Json.Decoder (List Silence)
listResponseDecoder =
    Json.at [ "data", "silences" ] (Json.list silenceDecoder)


silenceDecoder : Json.Decoder Silence
silenceDecoder =
    Json.map7 Silence
        (field "id" Json.int)
        (field "createdBy" Json.string)
        (field "comment" Json.string)
        (field "startsAt" stringtoISO8601)
        (field "endsAt" stringtoISO8601)
        (field "createdAt" stringtoISO8601)
        (field "matchers" (Json.list matcherDecoder))


matcherDecoder : Json.Decoder Matcher
matcherDecoder =
    Json.map3 Matcher
        (field "name" Json.string)
        (field "value" Json.string)
        (field "isRegex" Json.bool)
