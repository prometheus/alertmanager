module Views.SilenceView.Updates exposing (update)

import Alerts.Api
import Browser.Navigation as Navigation
import Silences.Api exposing (getSilence)
import Utils.Filter exposing (silencePreviewFilter)
import Utils.Types exposing (ApiData(..))
import Views.SilenceView.Types exposing (Model, SilenceViewMsg(..))


update : SilenceViewMsg -> Model -> String -> ( Model, Cmd SilenceViewMsg )
update msg model apiUrl =
    case msg of
        AlertGroupsPreview alerts ->
            ( { model | alerts = alerts }
            , Cmd.none
            )

        SetActiveAlert activeAlertId ->
            ( { model | activeAlertId = activeAlertId }
            , Cmd.none
            )

        SilenceFetched (Success silence) ->
            ( { model
                | silence = Success silence
                , alerts = Loading
              }
            , Alerts.Api.fetchAlerts
                apiUrl
                (silencePreviewFilter silence.matchers)
                |> Cmd.map AlertGroupsPreview
            )

        ConfirmDestroySilence ->
            ( { model | showConfirmationDialog = True }
            , Cmd.none
            )

        SilenceFetched silence ->
            ( { model | silence = silence, alerts = Initial }, Cmd.none )

        InitSilenceView silenceId ->
            ( { model | showConfirmationDialog = False }, getSilence apiUrl silenceId SilenceFetched )

        Reload silenceId ->
            ( { model | showConfirmationDialog = False }, Navigation.pushUrl model.key ("#/silences/" ++ silenceId) )
