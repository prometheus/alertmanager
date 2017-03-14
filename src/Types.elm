module Types exposing (..)

import Alerts.Types exposing (AlertGroup, AlertsMsg, Alert)
import Silences.Types exposing (SilencesMsg, Silence)
import Utils.Types exposing (ApiData, Filter)
import Time


type alias Model =
    { silences : ApiData (List Silence)
    , silence : ApiData Silence
    , alertGroups : ApiData (List AlertGroup)
    , route : Route
    , filter : Filter
    , currentTime : Time.Time
    }


type Msg
    = CreateSilenceFromAlert Alert
    | UpdateFilter Filter String
    | NavigateToAlerts Alerts.Types.Route
    | NavigateToSilences Silences.Types.Route
    | NavigateToStatus
    | Alerts AlertsMsg
    | Silences SilencesMsg
    | RedirectAlerts
    | NewUrl String
    | Noop
    | UpdateCurrentTime Time.Time


type Route
    = SilencesRoute Silences.Types.Route
    | AlertsRoute Alerts.Types.Route
    | StatusRoute
    | TopLevel
    | NotFound
