module Types exposing (..)

-- External Imports

import Alerts.Types exposing (AlertGroup, AlertsMsg, Alert)
import Silences.Types exposing (SilencesMsg, Silence)
import Http exposing (Error)
import ISO8601
import Time
import Utils.Types exposing (ApiData, Filter)


-- Internal Imports
-- Types


type alias Model =
    { silences : ApiData (List Silence)
    , silence : ApiData Silence
    , alertGroups : ApiData (List AlertGroup)
    , route : Route
    , filter : Filter
    }


type Msg
    = SilenceFetch (ApiData Silence)
    | SilencesFetch (ApiData (List Silence))
    | FetchSilences
    | FetchSilence String
    | NewSilence
    | EditSilence String
    | CreateSilenceFromAlert Alert
    | UpdateFilter Filter String
    | NavigateToAlerts Alerts.Types.Route
    | Alerts AlertsMsg
    | Silences SilencesMsg
    | RedirectAlerts
    | NewUrl String
    | Noop


type Route
    = SilencesRoute (Maybe String)
    | NewSilenceRoute
    | SilenceRoute String
    | EditSilenceRoute String
    | AlertsRoute Alerts.Types.Route
    | TopLevel
    | NotFound
