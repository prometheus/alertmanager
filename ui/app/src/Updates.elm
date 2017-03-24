module Updates exposing (update)

import Alerts.Api
import Navigation
import Silences.Types exposing (nullSilence)
import Task
import Types
    exposing
        ( Msg(..)
        , Model
        , Route(NotFoundRoute, SilenceFormEditRoute, SilenceFormNewRoute, SilenceRoute, StatusRoute, SilenceListRoute, AlertsRoute)
        )
import Utils.List
import Utils.Types
    exposing
        ( ApiResponse(Loading, Failure, Success)
        , Matcher
        , nullFilter
        )
import Views.AlertList.Updates
import Views.Silence.Types exposing (SilenceMsg(SilenceFetched, InitSilenceView))
import Views.Silence.Updates
import Views.SilenceForm.Types exposing (SilenceFormMsg(NewSilence, FetchSilence))
import Views.SilenceForm.Updates
import Views.SilenceList.Updates
import Views.Status.Types exposing (StatusMsg(InitStatusView))
import Views.Status.Updates
import String exposing (trim)


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        CreateSilenceFromAlert alert ->
            let
                silence =
                    { nullSilence | matchers = (List.map (\( k, v ) -> Matcher k v False) alert.labels) }
            in
                ( { model | silence = Success silence }, Cmd.none )

        PreviewSilence silence ->
            let
                s =
                    { silence | silencedAlertGroups = Loading }

                filter =
                    { nullFilter | text = Just <| Utils.List.mjoin s.matchers }
            in
                ( { model | silence = Success silence }, Alerts.Api.getAlertGroups filter AlertGroupsPreview )

        AlertGroupsPreview alertGroups ->
            let
                silence =
                    case model.silence of
                        Success sil ->
                            Success { sil | silencedAlertGroups = alertGroups }

                        Failure e ->
                            Failure e

                        Loading ->
                            Loading
            in
                ( { model | silence = silence }, Cmd.none )

        NavigateToAlerts alertsRoute ->
            let
                ( alertsMsg, filter ) =
                    (Views.AlertList.Updates.urlUpdate alertsRoute)

                ( alertGroups, cmd ) =
                    Views.AlertList.Updates.update alertsMsg model.alertGroups filter
            in
                ( { model | alertGroups = alertGroups, route = AlertsRoute alertsRoute, filter = filter }, cmd )

        NavigateToSilenceList maybeFilter ->
            let
                ( silencesMsg, filter ) =
                    (Views.SilenceList.Updates.urlUpdate maybeFilter)

                ( silences, silence, cmd ) =
                    Views.SilenceList.Updates.update silencesMsg model.silences model.silence filter
            in
                ( { model | silence = silence, silences = silences, route = SilenceListRoute maybeFilter, filter = filter }
                , cmd
                )

        NavigateToStatus ->
            ( { model | route = StatusRoute }, Task.perform identity (Task.succeed <| (MsgForStatus InitStatusView)) )

        NavigateToSilence silenceId ->
            let
                silenceMsg =
                    InitSilenceView silenceId

                cmd =
                    Task.perform identity (Task.succeed <| (MsgForSilence silenceMsg))
            in
                ( { model | route = (SilenceRoute silenceId) }, cmd )

        NavigateToSilenceFormNew ->
            ( { model | route = SilenceFormNewRoute }, Task.perform identity (Task.succeed <| (MsgForSilenceForm NewSilence)) )

        NavigateToSilenceFormEdit uuid ->
            ( { model | route = SilenceFormEditRoute uuid }, Task.perform identity (Task.succeed <| (FetchSilence uuid |> MsgForSilenceForm)) )

        NavigateToNotFound ->
            ( { model | route = NotFoundRoute }, Cmd.none )

        RedirectAlerts ->
            ( model, Task.perform NewUrl (Task.succeed "/#/alerts") )

        UpdateFilter filter text ->
            let
                t =
                    if trim text == "" then
                        Nothing
                    else
                        Just text
            in
                ( { model | filter = { filter | text = t } }, Cmd.none )

        NewUrl url ->
            ( model, Navigation.newUrl url )

        Noop ->
            ( model, Cmd.none )

        UpdateCurrentTime time ->
            ( { model | currentTime = time }, Cmd.none )

        MsgForStatus msg ->
            Views.Status.Updates.update msg model

        MsgForAlertList msg ->
            let
                ( alertGroups, cmd ) =
                    Views.AlertList.Updates.update msg model.alertGroups model.filter
            in
                ( { model | alertGroups = alertGroups }, cmd )

        MsgForSilenceList msg ->
            let
                ( silences, silence, cmd ) =
                    Views.SilenceList.Updates.update msg model.silences model.silence model.filter
            in
                ( { model | silences = silences, silence = silence }, cmd )

        MsgForSilence msg ->
            Views.Silence.Updates.update msg model

        MsgForSilenceForm msg ->
            Views.SilenceForm.Updates.update msg model
