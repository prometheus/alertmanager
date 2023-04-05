module Silences.Decoders exposing (create, destroy)

import Json.Decode as Json
import Utils.Types exposing (ApiData(..))


create : Json.Decoder String
create =
    Json.at [ "silenceID" ] Json.string


destroy : Json.Decoder String
destroy =
    Json.at [ "status" ] Json.string
