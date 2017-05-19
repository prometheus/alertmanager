module Types exposing (Model, Msg(..), Route(..))

import Alerts.Types exposing (AlertGroup, Alert)
import Views.AlertList.Types as AlertList exposing (AlertListMsg)
import Views.SilenceList.Types as SilenceList exposing (SilenceListMsg)
import Views.Silence.Types exposing (SilenceMsg)
import Views.SilenceForm.Types as SilenceForm exposing (SilenceFormMsg)
import Views.Status.Types exposing (StatusModel, StatusMsg)
import Silences.Types exposing (Silence)
import Utils.Types exposing (ApiData, Label)
import Utils.Filter exposing (Filter)


type alias Model =
    { silenceList : SilenceList.Model
    , silence : ApiData Silence
    , silenceForm : SilenceForm.Model
    , alertList : AlertList.Model
    , route : Route
    , filter : Filter
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
    | UpdateFilter String


type Route
    = AlertsRoute Filter
    | NotFoundRoute
    | SilenceFormEditRoute String
    | SilenceFormNewRoute Bool
    | SilenceListRoute Filter
    | SilenceRoute String
    | StatusRoute
    | TopLevelRoute
