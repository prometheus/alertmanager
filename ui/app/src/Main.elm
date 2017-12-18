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
        )
import Utils.Filter exposing (nullFilter)
import Views.SilenceForm.Types exposing (initSilenceForm)
import Views.Status.Types exposing (StatusModel, initStatusModel)
import Views.AlertList.Types exposing (initAlertList)
import Views.SilenceList.Types exposing (initSilenceList)
import Views.SilenceView.Types exposing (initSilenceView)
import Updates exposing (update)
import Utils.Api as Api
import Utils.Types exposing (ApiData(Loading))
import Json.Decode as Json


main : Program Json.Value Model Msg
main =
    Navigation.programWithFlags urlUpdate
        { init = init
        , update = update
        , view = Views.view
        , subscriptions = always Sub.none
        }


init : Json.Value -> Navigation.Location -> ( Model, Cmd Msg )
init flags location =
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

        prod =
            flags
                |> Json.decodeValue (Json.field "production" Json.bool)
                |> Result.withDefault False

        defaultCreator =
            flags
                |> Json.decodeValue (Json.field "defaultCreator" Json.string)
                |> Result.withDefault ""

        apiUrl =
            if prod then
                Api.makeApiUrl location.pathname
            else
                Api.makeApiUrl "http://localhost:9093/"

        libUrl =
            if prod then
                location.pathname
            else
                "/"
    in
        update (urlUpdate location)
            (Model
                initSilenceList
                initSilenceView
                initSilenceForm
                initAlertList
                route
                filter
                initStatusModel
                location.pathname
                apiUrl
                libUrl
                Loading
                Loading
                defaultCreator
            )


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

            SilenceFormNewRoute matchers ->
                NavigateToSilenceFormNew matchers

            AlertsRoute filter ->
                NavigateToAlerts filter

            StatusRoute ->
                NavigateToStatus

            TopLevelRoute ->
                RedirectAlerts

            NotFoundRoute ->
                NavigateToNotFound
