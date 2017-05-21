module Utils.Types exposing (..)

import Http
import Time


type ApiResponse e a
    = Initial
    | Loading
    | Failure e
    | Success a


type alias Matcher =
    { isRegex : Bool
    , name : String
    , value : String
    }


type alias Matchers =
    List Matcher


type alias Labels =
    List Label


type alias Label =
    ( String, String )


type alias ApiData a =
    ApiResponse Http.Error a


type alias Time =
    { t : Maybe Time.Time
    , s : String
    }


type alias Duration =
    { d : Maybe Time.Time
    , s : String
    }
