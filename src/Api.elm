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
baseUrl = "http://localhost:8080/api/v1"



getSilence : String -> Cmd Msg
getSilence sid =
  let
      url = String.join "/" [baseUrl, "sid", id]
  in
     Task.perform FetchFail FetchSucceed (Http.get responseDecoder url)


decodeApiResponse : Json.Decoder (List Silence)
decodeApiResponse =
  Json.list responseDecoder


responseDecoder : Json.Decoder Silence
responseDecoder =
  Json.at ["data"] Json.object3 Silence
    ("Title" := Json.string)
    ("FullPath" := Json.string)
    ("ApiMovie" := apiResponseDecoder)

silenceDecoder : Json.Decoder Silence
silenceDecoder =
  Json.object7 Silence
    ("id" := Json.string)
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


