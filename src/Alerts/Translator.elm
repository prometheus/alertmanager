module Alerts.Translator exposing (translator)

import Alerts.Types exposing (Msg(..), Alert, AlertsMsg, OutMsg(..))


type alias TranslationDictionary msg =
    { onInternalMessage : AlertsMsg -> msg
    , onSilenceFromAlert : Alert -> msg
    }


translator : TranslationDictionary parentMsg -> Msg -> parentMsg
translator { onInternalMessage, onSilenceFromAlert } msg =
    case msg of
        ForSelf internal ->
            onInternalMessage internal

        ForParent (SilenceFromAlert alert) ->
            onSilenceFromAlert alert
