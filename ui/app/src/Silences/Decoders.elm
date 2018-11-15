module Silences.Decoders exposing (create, destroy)

import Json.Decode as Json exposing (fail, field, succeed)
import Utils.Api exposing (andMap, iso8601Time)
import Utils.Types exposing (ApiData(..), Matcher, Time)


create : Json.Decoder String
create =
    Json.at [ "silenceID" ] Json.string


destroy : Json.Decoder String
destroy =
    Json.at [ "status" ] Json.string
