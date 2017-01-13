module Silences.Decoders exposing (..)

-- External

import Json.Decode as Json exposing (field)


-- Internal

import Utils.Api exposing (stringtoISO8601)
import Types exposing (Silence, Matcher)


show : Json.Decoder Silence
show =
    (Json.at [ "data" ] silence)


list : Json.Decoder (List Silence)
list =
    Json.at [ "data", "silences" ] (Json.list silence)


silence : Json.Decoder Silence
silence =
    Json.map7 Silence
        (field "id" Json.int)
        (field "createdBy" Json.string)
        (field "comment" Json.string)
        (field "startsAt" stringtoISO8601)
        (field "endsAt" stringtoISO8601)
        (field "createdAt" stringtoISO8601)
        (field "matchers" (Json.list matcher))


matcher : Json.Decoder Matcher
matcher =
    Json.map3 Matcher
        (field "name" Json.string)
        (field "value" Json.string)
        (field "isRegex" Json.bool)
