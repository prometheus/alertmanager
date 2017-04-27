module Updates exposing (update)

import Navigation
import Task
import Types
    exposing
        ( Msg(..)
        , Model
        , Route(NotFoundRoute, SilenceFormEditRoute, SilenceFormNewRoute, SilenceRoute, StatusRoute, SilenceListRoute, AlertsRoute)
        )
import Utils.Types
    exposing
        ( ApiResponse(Loading, Failure, Success)
        , Matcher
        )
import Views.AlertList.Updates
import Views.AlertList.Types exposing (AlertListMsg(FetchAlerts))
import Views.Silence.Types exposing (SilenceMsg(SilenceFetched, InitSilenceView))
import Views.SilenceList.Types exposing (SilenceListMsg(FetchSilences))
import Views.Silence.Updates
import Views.SilenceForm.Types exposing (SilenceFormMsg(NewSilenceFromMatchers, FetchSilence))
import Views.SilenceForm.Updates
import Views.SilenceList.Updates
import Views.Status.Types exposing (StatusMsg(InitStatusView))
import Views.Status.Updates
import String exposing (trim)


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        CreateSilenceFromAlert { labels } ->
            let
                matchers =
                    List.map (\( k, v ) -> Matcher k v False) labels

                ( silenceForm, cmd ) =
                    Views.SilenceForm.Updates.update (NewSilenceFromMatchers matchers) model.silenceForm
            in
                ( { model | silenceForm = silenceForm }, cmd )

        NavigateToAlerts filter ->
            let
                ( alertList, cmd ) =
                    Views.AlertList.Updates.update FetchAlerts model.alertList filter
            in
                ( { model | alertList = alertList, route = AlertsRoute filter, filter = filter }, cmd )

        NavigateToSilenceList filter ->
            let
                ( silences, silence, cmd ) =
                    Views.SilenceList.Updates.update FetchSilences model.silences model.silence filter
            in
                ( { model | silence = silence, silences = silences, route = SilenceListRoute filter, filter = filter }
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

        NavigateToSilenceFormNew keep ->
            ( { model | route = SilenceFormNewRoute keep }
            , if keep then
                Cmd.none
              else
                Task.perform (NewSilenceFromMatchers >> MsgForSilenceForm) (Task.succeed [])
            )

        NavigateToSilenceFormEdit uuid ->
            ( { model | route = SilenceFormEditRoute uuid }, Task.perform identity (Task.succeed <| (FetchSilence uuid |> MsgForSilenceForm)) )

        NavigateToNotFound ->
            ( { model | route = NotFoundRoute }, Cmd.none )

        RedirectAlerts ->
            ( model, Navigation.newUrl "/#/alerts" )

        UpdateFilter text ->
            let
                t =
                    if trim text == "" then
                        Nothing
                    else
                        Just text

                prevFilter =
                    model.filter
            in
                ( { model | filter = { prevFilter | text = t } }, Cmd.none )

        Noop ->
            ( model, Cmd.none )

        UpdateCurrentTime time ->
            ( { model | currentTime = time }, Cmd.none )

        MsgForStatus msg ->
            Views.Status.Updates.update msg model

        MsgForAlertList msg ->
            let
                ( alertList, cmd ) =
                    Views.AlertList.Updates.update msg model.alertList model.filter
            in
                ( { model | alertList = alertList }, cmd )

        MsgForSilenceList msg ->
            let
                ( silences, silence, cmd ) =
                    Views.SilenceList.Updates.update msg model.silences model.silence model.filter
            in
                ( { model | silences = silences, silence = silence }, cmd )

        MsgForSilence msg ->
            Views.Silence.Updates.update msg model

        MsgForSilenceForm msg ->
            let
                ( silenceForm, cmd ) =
                    Views.SilenceForm.Updates.update msg model.silenceForm
            in
                ( { model | silenceForm = silenceForm }, cmd )
