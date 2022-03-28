module Utils.Types exposing (ApiData(..), Label, Labels, Matcher)


type ApiData a
    = Initial
    | Loading
    | Failure String
    | Success a


type alias Matcher =
    { isRegex : Bool
    , isEqual : Maybe Bool
    , name : String
    , value : String
    }


type alias Matchers =
    List Matcher


type alias Labels =
    List Label


type alias Label =
    ( String, String )
