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
            String.join "/" [ baseUrl, "silences" ]
    in
        Task.perform FetchFail SilencesFetchSucceed (Http.get listResponseDecoder url)


getSilence : String -> Cmd Msg
getSilence id =
    let
        url =
            String.join "/" [ baseUrl, "silence", id ]
    in
        Task.perform FetchFail SilenceFetchSucceed (Http.get showResponseDecoder url)


-- Make these generic when I've gotten to Alerts
showResponseDecoder : Json.Decoder Silence
showResponseDecoder =
    (Json.at [ "data" ] silenceDecoder)

listResponseDecoder : Json.Decoder (List Silence)
listResponseDecoder =
    Json.at [ "data", "silences" ] (Json.list silenceDecoder)


silenceDecoder : Json.Decoder Silence
silenceDecoder =
    Json.object7 Silence
        ("id" := Json.int)
        ("createdBy" := Json.string)
        ("comment" := Json.string)
        ("startsAt" := Json.string)
        ("endsAt" := Json.string)
        ("createdAt" := Json.string)
        ("matchers" := (Json.list matcherDecoder))


matcherDecoder : Json.Decoder Matcher
matcherDecoder =
    Json.object3 Matcher
        ("name" := Json.string)
        ("value" := Json.string)
        ("isRegex" := Json.bool)
