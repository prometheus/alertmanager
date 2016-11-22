module Api exposing (..)

-- External Imports

import Http
import Json.Decode as Json exposing (..)
import Task
import String


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


getSilence : String -> Cmd Msg
getSilence id =
    let
        url =
            String.join "/" [ baseUrl, "silence", id ]
    in
        Http.send SilenceFetch (Http.get url showResponseDecoder)



-- Make these generic when I've gotten to Alerts


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
