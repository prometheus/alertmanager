module Main exposing (main)

import Navigation
import Parsing
import Views
import Types
    exposing
        ( Route(..)
        , Msg
            ( NavigateToSilenceList
            , NavigateToSilenceView
            , NavigateToSilenceFormEdit
            , NavigateToSilenceFormNew
            , NavigateToAlerts
            , NavigateToNotFound
            , NavigateToStatus
            , RedirectAlerts
            )
        , Model
        , Flags
        )
import Utils.Filter exposing (nullFilter)
import Views.SilenceForm.Types exposing (initSilenceForm)
import Views.Status.Types exposing (StatusModel, initStatusModel)
import Views.AlertList.Types exposing (initAlertList)
import Views.SilenceList.Types exposing (initSilenceList)
import Views.SilenceView.Types exposing (initSilenceView)
import Updates exposing (update)
import Utils.Api as Api


main : Program Flags Model Msg
main =
    Navigation.programWithFlags urlUpdate
        { init = init
        , update = update
        , view = Views.view
        , subscriptions = always Sub.none
        }


init : Flags -> Navigation.Location -> ( Model, Cmd Msg )
init { baseUrl } location =
    let
        route =
            Parsing.urlParser location

        filter =
            case route of
                AlertsRoute filter ->
                    filter

                SilenceListRoute filter ->
                    filter

                _ ->
                    nullFilter

        apiUrl =
            Api.makeApiUrl baseUrl
    in
        update (urlUpdate location) (Model initSilenceList initSilenceView initSilenceForm initAlertList route filter initStatusModel baseUrl apiUrl)


urlUpdate : Navigation.Location -> Msg
urlUpdate location =
    let
        route =
            Parsing.urlParser location
    in
        case route of
            SilenceListRoute maybeFilter ->
                NavigateToSilenceList maybeFilter

            SilenceViewRoute silenceId ->
                NavigateToSilenceView silenceId

            SilenceFormEditRoute silenceId ->
                NavigateToSilenceFormEdit silenceId

            SilenceFormNewRoute keep ->
                NavigateToSilenceFormNew keep

            AlertsRoute filter ->
                NavigateToAlerts filter

            StatusRoute ->
                NavigateToStatus

            TopLevelRoute ->
                RedirectAlerts

            NotFoundRoute ->
                NavigateToNotFound
