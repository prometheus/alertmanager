module Translators exposing (..)

import Alerts.Types
import Alerts.Translator
import Silences.Types
import Silences.Translator
import Types exposing (Msg(Alerts, CreateSilenceFromAlert, Silences, UpdateFilter, NewUrl))


alertTranslator : Alerts.Types.Msg -> Msg
alertTranslator =
    Alerts.Translator.translator
        { onInternalMessage = Alerts
        , onSilenceFromAlert = CreateSilenceFromAlert
        , onUpdateFilter = UpdateFilter
        , onNewUrl = NewUrl
        }


silenceTranslator : Silences.Types.Msg -> Msg
silenceTranslator =
    Silences.Translator.translator
        { onFormMsg = Silences
        , onNewUrl = NewUrl
        , onUpdateFilter = UpdateFilter
        }
