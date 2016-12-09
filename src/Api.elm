module Api exposing (..)

-- External Imports

import Http
import Json.Decode as Json exposing (..)
import Task
import String
import Date exposing (..)


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
        (Json.at [ "blocks" ] <| Json.maybe <| Json.list blockDecoder)
        (Json.at [ "labels" ] (Json.keyValuePairs Json.string))


blockDecoder : Json.Decoder Block
blockDecoder =
    Json.map Block
        (field "alerts" <| Json.list alertDecoder)


alertDecoder : Json.Decoder Alert
alertDecoder =
    Json.map6 Alert
        (field "annotations" (Json.keyValuePairs Json.string))
        (field "labels" (Json.keyValuePairs Json.string))
        (field "inhibited" Json.bool)
        (Json.maybe (field "silenced" Json.int))
        (field "startsAt" stringToDate)
        (field "generatorURL" Json.string)


stringToDate : Decoder Date.Date
stringToDate =
    Json.string
        |> andThen
            (\val ->
                case Date.fromString val of
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
        (field "startsAt" Json.string)
        (field "endsAt" Json.string)
        (field "createdAt" Json.string)
        (field "matchers" (Json.list matcherDecoder))


matcherDecoder : Json.Decoder Matcher
matcherDecoder =
    Json.map3 Matcher
        (field "name" Json.string)
        (field "value" Json.string)
        (field "isRegex" Json.bool)
