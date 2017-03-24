module Views.Silence.Types exposing (SilenceMsg(..))

import Silences.Types exposing (Silence, SilenceId)
import Utils.Types exposing (ApiData)


type SilenceMsg
    = FetchSilence String
    | SilenceFetched (ApiData Silence)
    | InitSilenceView SilenceId
