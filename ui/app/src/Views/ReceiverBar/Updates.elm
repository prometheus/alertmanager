module Views.ReceiverBar.Updates exposing (update, fetchReceivers)

import Views.ReceiverBar.Types exposing (Model, Msg(..))
import Utils.Types exposing (ApiData(Success))
import Utils.Filter exposing (Filter, generateQueryString, stringifyGroup, parseGroup)
import Navigation
import Dom
import Task
import Alerts.Api as Api
import Utils.Match exposing (jaroWinkler)


update : String -> Filter -> Msg -> Model -> ( Model, Cmd Msg )
update url filter msg model =
    case msg of
        ReceiversFetched (Success receivers) ->
            ( { model | receivers = receivers }, Cmd.none )

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
            , Navigation.newUrl
                (url
                    ++ generateQueryString
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
