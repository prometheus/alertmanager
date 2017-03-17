module Types exposing (..)

import Alerts.Types exposing (AlertGroup, AlertsMsg, Alert)
import Silences.Types exposing (SilencesMsg, Silence)
import Status.Types exposing (StatusModel, StatusMsg)
import Utils.Types exposing (ApiData, Filter)
import Time


type alias Model =
    { silences : ApiData (List Silence)
    , silence : ApiData Silence
    , alertGroups : ApiData (List AlertGroup)
    , route : Route
    , filter : Filter
    , currentTime : Time.Time
    , status : StatusModel
    }


type Msg
    = CreateSilenceFromAlert Alert
    | PreviewSilence Silence
    | AlertGroupsPreview (ApiData (List AlertGroup))
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
    | MsgForStatus StatusMsg


type Route
    = SilencesRoute Silences.Types.Route
    | AlertsRoute Alerts.Types.Route
    | StatusRoute
    | TopLevel
    | NotFound
