module Example exposing (..)

import Json.Decode as Decode exposing (Decoder, float, int, string)
import Json.Decode.Pipeline exposing (hardcoded, optional, required)


type alias User =
    { id : Int
    , name : String
    , percentExcited : Float
    }


userDecoder : Decoder User
userDecoder =
    Decode.succeed User
        |> required "id" int
        |> optional "name" string "(fallback if name not present)"
        |> hardcoded 1.0
