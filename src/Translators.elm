module Translators exposing (..)

import Alerts.Types
import Alerts.Translator
import Silences.Types
import Silences.Translator
import Types exposing (Msg(Alerts, CreateSilenceFromAlert, Silences))


alertTranslator : Alerts.Types.Msg -> Msg
alertTranslator =
    Alerts.Translator.translator
        { onInternalMessage = Alerts
        , onSilenceFromAlert = CreateSilenceFromAlert
        }


silenceTranslator : Silences.Types.Msg -> Msg
silenceTranslator =
    Silences.Translator.translator
        { onFormMsg = Silences
        }
