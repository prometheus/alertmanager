module Views.SilenceList.Views exposing (..)

import Html exposing (..)
import Html.Attributes exposing (..)
import Views.SilenceList.Types exposing (SilenceListMsg(..), Model)
import Views.SilenceList.SilenceView
import Silences.Types exposing (Silence, State(..), stateToString)
import Utils.Types exposing (Matcher, ApiData(..))
import Utils.Api exposing (withDefault)
import Utils.Views exposing (iconButtonMsg, checkbox, textField, formInput, formField, buttonLink, error, loading)
import Types exposing (Msg(UpdateFilter, MsgForSilenceList, Noop))
import Views.FilterBar.Views as FilterBar
import Utils.String as StringUtils


view : Model -> Html Msg
view { filterBar, tab, silences } =
    div []
        [ div [ class "mb-4" ]
            [ label [ class "mb-2", for "filter-bar-matcher" ] [ text "Filter" ]
            , Html.map (MsgForFilterBar >> MsgForSilenceList) (FilterBar.view filterBar)
            ]
        , ul [ class "nav nav-tabs mb-4" ]
            (List.map (tabView tab) (groupSilencesByState (withDefault [] silences)))
        , case silences of
            Success sils ->
                silencesView (filterSilencesByState tab sils)

            Failure msg ->
                error msg

            _ ->
                loading
        ]


tabView : State -> ( State, List a ) -> Html Msg
tabView currentState ( state, silences ) =
    Utils.Views.tab state currentState (SetTab >> MsgForSilenceList) <|
        case List.length silences of
            0 ->
                [ text (StringUtils.capitalizeFirst (stateToString state)) ]

            n ->
                [ text (StringUtils.capitalizeFirst (stateToString state))
                , span
                    [ class "badge badge-pillow badge-default align-text-top ml-2" ]
                    [ text (toString n) ]
                ]


silencesView : List Silence -> Html Msg
silencesView silences =
    if List.isEmpty silences then
        div [] [ text "No silences found" ]
    else
        ul [ class "list-group" ]
            (List.map Views.SilenceList.SilenceView.view silences)


groupSilencesByState : List Silence -> List ( State, List Silence )
groupSilencesByState silences =
    List.map (\state -> ( state, filterSilencesByState state silences )) states


states : List State
states =
    [ Active, Pending, Expired ]


filterSilencesByState : State -> List Silence -> List Silence
filterSilencesByState state =
    List.filter (.status >> .state >> (==) state)
