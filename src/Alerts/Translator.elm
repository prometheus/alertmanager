module Alerts.Translator exposing (translator)

import Alerts.Types exposing (Msg(..), AlertGroup, Alert, AlertsMsg, OutMsg(..))
import Utils.Types exposing (ApiData, Filter)


type alias TranslationDictionary msg =
    { onInternalMessage : AlertsMsg -> msg
    , onSilenceFromAlert : Alert -> msg
    , onUpdateFilter : Filter -> String -> msg
    , onNewUrl : String -> msg
    , onAlertGroupsPreview : ApiData (List AlertGroup) -> msg
    }


translator : TranslationDictionary parentMsg -> Msg -> parentMsg
translator { onInternalMessage, onSilenceFromAlert, onUpdateFilter, onNewUrl, onAlertGroupsPreview } msg =
    case msg of
        ForSelf internal ->
            onInternalMessage internal

        ForParent (SilenceFromAlert alert) ->
            onSilenceFromAlert alert

        ForParent (UpdateFilter filter string) ->
            onUpdateFilter filter string

        ForParent (NewUrl string) ->
            onNewUrl string

        ForParent (AlertGroupsPreview groups) ->
            onAlertGroupsPreview groups
