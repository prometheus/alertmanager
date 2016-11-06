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
    "/api/v1"


getSilences : Cmd Msg
getSilences =
    let
        url =
            String.join "/" [ baseUrl, "silences" ]
    in
        Task.perform FetchFail SilencesFetchSucceed (Http.get responseDecoder url)


decodeApiResponse : Json.Decoder (List Silence)
decodeApiResponse =
    Json.list silenceDecoder


responseDecoder : Json.Decoder (List Silence)
responseDecoder =
    Json.at [ "data", "silences" ] decodeApiResponse


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
