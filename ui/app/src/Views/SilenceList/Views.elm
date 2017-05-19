module Views.SilenceList.Views exposing (..)

import Html exposing (..)
import Html.Attributes exposing (..)
import Views.SilenceList.Types exposing (SilenceListMsg(..), Model)
import Views.Shared.SilenceBase
import Silences.Types exposing (Silence, State(..), stateToString)
import Utils.Types exposing (Matcher, ApiResponse(..), ApiData)
import Utils.Views exposing (iconButtonMsg, checkbox, textField, formInput, formField, buttonLink, error, loading)
import Types exposing (Msg(UpdateFilter, MsgForSilenceList, Noop))
import Views.FilterBar.Views as FilterBar
import Utils.String as StringUtils


view : Model -> Html Msg
view model =
    case model.silences of
        Success sils ->
            div []
                [ Html.map (MsgForFilterBar >> MsgForSilenceList) (FilterBar.view model.filterBar)
                , a [ class "mb-4 btn btn-primary", href "#/silences/new" ] [ text "New Silence" ]
                , silenceListView sils
                ]

        Initial ->
            loading

        Loading ->
            loading

        Failure msg ->
            error msg


silenceListView : List Silence -> Html Msg
silenceListView silences =
    div [] <|
        List.map silenceGroupView <|
            groupSilencesByState silences


silenceGroupView : ( State, List Silence ) -> Html Msg
silenceGroupView ( state, silences ) =
    let
        silencesView =
            if List.isEmpty silences then
                div [] [ text "No silences found" ]
            else
                ul [ class "list-group" ]
                    (List.map silenceView silences)
    in
        div [ class "mb-4" ]
            [ h3 [] [ text <| StringUtils.capitalizeFirst <| stateToString state ]
            , silencesView
            ]


silenceView : Silence -> Html Msg
silenceView silence =
    li
        [ class "list-group-item p-0 list-item" ]
        [ Views.Shared.SilenceBase.view silence
        ]


groupSilencesByState : List Silence -> List ( State, List Silence )
groupSilencesByState silences =
    List.map (\state -> ( state, filterSilencesByState state silences )) states


states : List State
states =
    [ Active, Pending, Expired ]



-- TODO: Replace this with Utils.List.groupBy


filterSilencesByState : State -> List Silence -> List Silence
filterSilencesByState state =
    List.filter (\{ status } -> status.state == state)
