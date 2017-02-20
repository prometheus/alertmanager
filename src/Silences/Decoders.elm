module Silences.Decoders exposing (..)

import Json.Decode as Json exposing (field)
import Utils.Api exposing (iso8601Time)
import Silences.Types exposing (Silence)
import Utils.Types exposing (Matcher)


show : Json.Decoder Silence
show =
    (Json.at [ "data" ] silence)


list : Json.Decoder (List Silence)
list =
    Json.at [ "data", "silences" ] (Json.list silence)


create : Json.Decoder Int
create =
    (Json.at [ "data", "silenceId" ] Json.int)



-- This should just be the ID


destroy : Json.Decoder String
destroy =
    (Json.at [ "status" ] Json.string)


silence : Json.Decoder Silence
silence =
    Json.map7 Silence
        (field "id" Json.int)
        (field "createdBy" Json.string)
        (field "comment" Json.string)
        (iso8601Time "startsAt")
        (iso8601Time "endsAt")
        (iso8601Time "createdAt")
        (field "matchers" (Json.list matcher))


matcher : Json.Decoder Matcher
matcher =
    Json.map3 Matcher
        (field "name" Json.string)
        (field "value" Json.string)
        (field "isRegex" Json.bool)
