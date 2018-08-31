module Utils.Types exposing (ApiData(..), Duration, Label, Labels, Matcher, Matchers, Time)

import Time


type ApiData a
    = Initial
    | Loading
    | Failure String
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


type alias Time =
    { t : Maybe Time.Time
    , s : String
    }


type alias Duration =
    { d : Maybe Time.Time
    , s : String
    }
