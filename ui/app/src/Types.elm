module Types exposing (Model, Msg(..), Route(..))

import Alerts.Types exposing (AlertGroup, Alert)
import Views.AlertList.Types exposing (AlertListMsg)
import Views.SilenceList.Types exposing (SilenceListMsg)
import Views.Silence.Types exposing (SilenceMsg)
import Views.SilenceForm.Types as SilenceForm exposing (SilenceFormMsg)
import Views.Status.Types exposing (StatusModel, StatusMsg)
import Silences.Types exposing (Silence)
import Utils.Types exposing (ApiData, Filter)
import Time


type alias Model =
    { silences : ApiData (List Silence)
    , silence : ApiData Silence
    , silenceForm : SilenceForm.Model
    , alertGroups : ApiData (List AlertGroup)
    , route : Route
    , filter : Filter
    , currentTime : Time.Time
    , status : StatusModel
    }


type Msg
    = CreateSilenceFromAlert Alert
    | MsgForAlertList AlertListMsg
    | MsgForSilence SilenceMsg
    | MsgForSilenceForm SilenceFormMsg
    | MsgForSilenceList SilenceListMsg
    | MsgForStatus StatusMsg
    | NavigateToAlerts Filter
    | NavigateToNotFound
    | NavigateToSilence String
    | NavigateToSilenceFormEdit String
    | NavigateToSilenceFormNew Bool
    | NavigateToSilenceList Filter
    | NavigateToStatus
    | Noop
    | RedirectAlerts
    | UpdateCurrentTime Time.Time
    | UpdateFilter Filter String


type Route
    = AlertsRoute Filter
    | NotFoundRoute
    | SilenceFormEditRoute String
    | SilenceFormNewRoute Bool
    | SilenceListRoute Filter
    | SilenceRoute String
    | StatusRoute
    | TopLevelRoute
