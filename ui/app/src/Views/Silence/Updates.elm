module Views.Silence.Updates exposing (update)

import Views.Silence.Types exposing (SilenceMsg(..))
import Types exposing (Model, Msg(MsgForSilence))
import Silences.Api exposing (getSilence)
import Utils.Types exposing (ApiResponse(Success))
import Task
import Types exposing (Msg(PreviewSilence))


update : SilenceMsg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        FetchSilence id ->
            ( model, getSilence id (SilenceFetched >> MsgForSilence) )

        SilenceFetched (Success sil) ->
            ( { model | silence = Success sil }
            , Task.perform PreviewSilence (Task.succeed sil)
            )

        SilenceFetched silence ->
            ( { model | silence = silence }, Cmd.none )

        InitSilenceView silenceId ->
            ( model, getSilence silenceId (SilenceFetched >> MsgForSilence) )
