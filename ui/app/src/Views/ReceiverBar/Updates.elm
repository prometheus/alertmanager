module Views.ReceiverBar.Updates exposing (fetchReceivers, update)

import Alerts.Api as Api
import Browser.Dom as Dom
import Browser.Navigation as Navigation
import Task
import Utils.Filter exposing (Filter)
import Utils.Match exposing (jaroWinkler)
import Utils.Types exposing (ApiData(..))
import Views.ReceiverBar.Types exposing (Model, Msg(..), apiReceiverToReceiver)


update : String -> Filter -> Msg -> Model -> ( Model, Cmd Msg )
update url filter msg model =
    case msg of
        ReceiversFetched (Success receivers) ->
            ( { model | receivers = List.map apiReceiverToReceiver receivers }, Cmd.none )

        ReceiversFetched _ ->
            ( model, Cmd.none )

        EditReceivers ->
            ( { model
                | showReceivers = True
                , fieldText = ""
                , matches =
                    model.receivers
                        |> List.take 10
                        |> (::) { name = "All", regex = "" }
                , selectedReceiver = Nothing
              }
            , Dom.focus "receiver-field" |> Task.attempt (always Noop)
            )

        ResultsHovered resultsHovered ->
            ( { model | resultsHovered = resultsHovered }, Cmd.none )

        UpdateReceiver receiver ->
            let
                matches =
                    model.receivers
                        |> List.sortBy (.name >> jaroWinkler receiver)
                        |> List.reverse
                        |> List.take 10
                        |> (::) { name = "All", regex = "" }
            in
            ( { model
                | fieldText = receiver
                , matches = matches
              }
            , Cmd.none
            )

        BlurReceiverField ->
            ( { model | showReceivers = False }, Cmd.none )

        Select maybeReceiver ->
            ( { model | selectedReceiver = maybeReceiver }, Cmd.none )

        FilterByReceiver regex ->
            ( { model | showReceivers = False, resultsHovered = False }
            , Navigation.pushUrl model.key
                (Utils.Filter.toUrl url
                    { filter
                        | receiver =
                            if regex == "" then
                                Nothing

                            else
                                Just regex
                    }
                )
            )

        Noop ->
            ( model, Cmd.none )


fetchReceivers : String -> Cmd Msg
fetchReceivers =
    Api.fetchReceivers >> Cmd.map ReceiversFetched
