module Utils.Keyboard exposing (keys, onKeyDown, onKeyUp)

import Html exposing (Attribute)
import Html.Events exposing (keyCode, on)
import Json.Decode as Json


keys :
    { backspace : Int
    , enter : Int
    , up : Int
    , down : Int
    }
keys =
    { backspace = 8
    , enter = 13
    , up = 38
    , down = 40
    }


onKeyDown : (Int -> msg) -> Attribute msg
onKeyDown tagger =
    on "keydown" (Json.map tagger keyCode)


onKeyUp : (Int -> msg) -> Attribute msg
onKeyUp tagger =
    on "keyup" (Json.map tagger keyCode)
