module Types exposing (..)


-- External Imports
import Http exposing (Error)


-- Internal Imports


-- Types


type alias Model =
  { silences : List Silence
  , silence : Silence
  , alerts : List Alert
  , alert : Alert
  , route : Route
  }

type Silence =
    { id : Int
    , createdBy : String
    , comment : String
    , startsAt : String
    , endsAt : String
    , createdAt : String
    , matchers : List Matcher
    }

-- TODO: Implement Alert.
type Alert = {}

-- TODO: Implement Matcher.
type Matcher = {}

type Msg
  = SilenceFetchSucceed Silence
  | SilencesFetchSucceed (List Silence)
  | AlertFetchSucceed Alert
  | AlertsFetchSucceed (List Alert)
  | FetchFail Http.Error

type Route
  = Silences
  | Silence String
  | Alerts
  | Alert String
  | NotFound

